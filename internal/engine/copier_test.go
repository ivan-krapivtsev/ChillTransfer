package engine

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic tests. It is safe
// for concurrent use: Run polls Now() from its goroutine while the test
// advances time.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t
}

// scriptGovernor is a scripted Governor: each Tick returns the next scripted
// allow result; once the script is exhausted it keeps returning defaultAllow.
type scriptGovernor struct {
	mu           sync.Mutex
	script       []bool
	defaultAllow bool
	pauseState   State
	ticks        int
	dts          []time.Duration
}

func (g *scriptGovernor) Tick(dt time.Duration) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	allow := g.defaultAllow
	if g.ticks < len(g.script) {
		allow = g.script[g.ticks]
	}
	g.ticks++
	g.dts = append(g.dts, dt)
	return allow, "fake"
}

func (g *scriptGovernor) PauseState() State { return g.pauseState }

func (g *scriptGovernor) tickCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ticks
}

// waitFor polls cond until true or the real-time timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for condition", timeout)
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func mustChtimes(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(got) != len(want) || string(got) != string(want) {
		t.Errorf("%s content = %q, want %q", path, got, want)
	}
}

// second-granularity timestamps: portable across APFS/NTFS/ext4/exFAT.
var (
	when1 = time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	when2 = when1.Add(time.Hour)
)

func TestCopierRecursiveFolderCopyPreservesStructureAndMtimes(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	root := filepath.Join(src, "tree")
	writeFile(t, filepath.Join(root, "a.txt"), []byte("hello"))
	writeFile(t, filepath.Join(root, "sub", "b.txt"), []byte("world!"))

	mustChtimes(t, filepath.Join(root, "a.txt"), when1)
	mustChtimes(t, filepath.Join(root, "sub", "b.txt"), when2)
	srcRootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat source root: %v", err)
	}

	q := NewQueue()
	if err := q.Add([]string{root}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := q.Items()[0].SizeBytes; got != 11 { // "hello" + "world!"
		t.Fatalf("queue item size = %d, want 11", got)
	}

	outRoot := filepath.Join(dest, "out")
	c := NewCopier(q, outRoot, nil, &fakeClock{t: time.Unix(0, 0)})
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snap := c.Snapshot()
	if snap.State != StateDone {
		t.Errorf("state = %s, want done", snap.State)
	}
	assertClose(t, snap.OverallPercent, 100)

	// Structure and content preserved.
	assertFileContent(t, filepath.Join(outRoot, "tree", "a.txt"), []byte("hello"))
	assertFileContent(t, filepath.Join(outRoot, "tree", "sub", "b.txt"), []byte("world!"))

	// File mtimes preserved (FR-3).
	aInfo, err := os.Stat(filepath.Join(outRoot, "tree", "a.txt"))
	if err != nil {
		t.Fatalf("stat dest a.txt: %v", err)
	}
	bInfo, err := os.Stat(filepath.Join(outRoot, "tree", "sub", "b.txt"))
	if err != nil {
		t.Fatalf("stat dest b.txt: %v", err)
	}
	if !aInfo.ModTime().Equal(when1) || !bInfo.ModTime().Equal(when2) {
		t.Errorf("dest mtimes = %s / %s, want %s / %s", aInfo.ModTime(), bInfo.ModTime(), when1, when2)
	}

	// Mode preserved. Windows has no Unix permission bits (Chmod only toggles
	// read-only), so the exact-mode check applies to Unix platforms only.
	if runtime.GOOS != "windows" {
		if got := aInfo.Mode().Perm(); got != 0o644 {
			t.Errorf("dest mode = %o, want 644", got)
		}
	}

	// Directory mtime preserved (best-effort, FR-3).
	destRootInfo, err := os.Stat(filepath.Join(outRoot, "tree"))
	if err != nil {
		t.Fatalf("stat dest root: %v", err)
	}
	if !destRootInfo.ModTime().Equal(srcRootInfo.ModTime()) {
		t.Errorf("dest dir mtime = %s, want %s", destRootInfo.ModTime(), srcRootInfo.ModTime())
	}

	items := q.Items()
	if items[0].Status != ItemDone {
		t.Errorf("item status = %s, want done", items[0].Status)
	}
	if got := q.ProcessedBytes(); got != 11 {
		t.Errorf("ProcessedBytes() = %d, want 11", got)
	}
}

func TestCopierSkipsExistingFileWhenSizeAndMtimeMatch(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "f.txt"), []byte("AAAA"))
	mustChtimes(t, filepath.Join(src, "f.txt"), when1)

	// Dest exists with matching size+mtime but different content: must be skipped.
	writeFile(t, filepath.Join(dest, "f.txt"), []byte("BBBB"))
	mustChtimes(t, filepath.Join(dest, "f.txt"), when1)

	q := NewQueue()
	if err := q.Add([]string{filepath.Join(src, "f.txt")}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c := NewCopier(q, dest, nil, &fakeClock{t: time.Unix(0, 0)})
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	items := q.Items()
	if items[0].Status != ItemSkipped {
		t.Errorf("item status = %s, want skipped", items[0].Status)
	}
	assertFileContent(t, filepath.Join(dest, "f.txt"), []byte("BBBB")) // untouched
	if got := q.ProcessedBytes(); got != 4 {
		t.Errorf("ProcessedBytes() = %d, want 4 (skipped counts as processed)", got)
	}
	snap := c.Snapshot()
	assertClose(t, snap.OverallPercent, 100)
}

func TestCopierRecopiesWhenMtimeDiffers(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "f.txt"), []byte("AAAA"))
	mustChtimes(t, filepath.Join(src, "f.txt"), when1)

	// Same size, different mtime: re-copied even with Overwrite=false.
	writeFile(t, filepath.Join(dest, "f.txt"), []byte("BBBB"))
	mustChtimes(t, filepath.Join(dest, "f.txt"), when2)

	q := NewQueue()
	if err := q.Add([]string{filepath.Join(src, "f.txt")}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c := NewCopier(q, dest, nil, &fakeClock{t: time.Unix(0, 0)})
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	items := q.Items()
	if items[0].Status != ItemDone {
		t.Errorf("item status = %s, want done", items[0].Status)
	}
	assertFileContent(t, filepath.Join(dest, "f.txt"), []byte("AAAA"))
	info, err := os.Stat(filepath.Join(dest, "f.txt"))
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if !info.ModTime().Equal(when1) {
		t.Errorf("dest mtime = %s, want %s (copied from source)", info.ModTime(), when1)
	}
}

func TestCopierOverwriteMode(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "f.txt"), []byte("AAAA"))
	mustChtimes(t, filepath.Join(src, "f.txt"), when1)

	// Size+mtime match but Overwrite=true: re-copied.
	writeFile(t, filepath.Join(dest, "f.txt"), []byte("CCCC"))
	mustChtimes(t, filepath.Join(dest, "f.txt"), when1)

	q := NewQueue()
	if err := q.Add([]string{filepath.Join(src, "f.txt")}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c := NewCopier(q, dest, nil, &fakeClock{t: time.Unix(0, 0)})
	c.Overwrite = true
	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	items := q.Items()
	if items[0].Status != ItemDone {
		t.Errorf("item status = %s, want done", items[0].Status)
	}
	assertFileContent(t, filepath.Join(dest, "f.txt"), []byte("AAAA"))
}

// destCollisionSetup creates src with good.txt + bad.txt and pre-creates
// dest/bad.txt as a directory so copying bad.txt fails deterministically.
func destCollisionSetup(t *testing.T) (goodSrc, badSrc string, q *Queue, c *Copier) {
	t.Helper()
	src := t.TempDir()
	dest := t.TempDir()
	goodSrc = filepath.Join(src, "good.txt")
	badSrc = filepath.Join(src, "bad.txt")
	writeFile(t, goodSrc, []byte("ok"))
	writeFile(t, badSrc, []byte("boom"))
	if err := os.MkdirAll(filepath.Join(dest, "bad.txt"), 0o755); err != nil {
		t.Fatalf("mkdir dest collision: %v", err)
	}

	q = NewQueue()
	if err := q.Add([]string{goodSrc, badSrc}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c = NewCopier(q, dest, nil, &fakeClock{t: time.Unix(0, 0)})
	return goodSrc, badSrc, q, c
}

func TestCopierPerFileErrorContinuesByDefault(t *testing.T) {
	goodSrc, _, q, c := destCollisionSetup(t)
	dest := c.Dest

	if err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run (continue mode) = %v, want nil", err)
	}

	snap := c.Snapshot()
	if snap.State != StateDone {
		t.Errorf("state = %s, want done (queue fully processed)", snap.State)
	}
	items := q.Items()
	if items[0].Status != ItemDone || items[1].Status != ItemFailed {
		t.Errorf("item statuses = [%s %s], want [done failed]", items[0].Status, items[1].Status)
	}
	assertFileContent(t, filepath.Join(dest, "good.txt"), []byte("ok")) // next item was reached
	if got := q.ProcessedBytes(); got != 6 {                            // 2 ("ok") + 4 ("boom", failed counts)
		t.Errorf("ProcessedBytes() = %d, want 6", got)
	}
	_ = goodSrc
}

func TestCopierStopOnError(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	good := filepath.Join(src, "good.txt")
	bad := filepath.Join(src, "bad.txt")
	third := filepath.Join(src, "third.txt")
	writeFile(t, good, []byte("ok"))
	writeFile(t, bad, []byte("boom"))
	writeFile(t, third, []byte("tail"))
	if err := os.MkdirAll(filepath.Join(dest, "bad.txt"), 0o755); err != nil {
		t.Fatalf("mkdir dest collision: %v", err)
	}

	q := NewQueue()
	if err := q.Add([]string{good, bad, third}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c := NewCopier(q, dest, nil, &fakeClock{t: time.Unix(0, 0)})
	c.StopOnError = true

	err := c.Run(context.Background())
	if err == nil {
		t.Fatal("Run (stop-on-error) = nil, want the file error")
	}

	snap := c.Snapshot()
	if snap.State != StateError {
		t.Errorf("state = %s, want error", snap.State)
	}
	items := q.Items()
	want := []ItemStatus{ItemDone, ItemFailed, ItemPending}
	for i, w := range want {
		if items[i].Status != w {
			t.Errorf("item %d status = %s, want %s", i, items[i].Status, w)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "third.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("third file must not be copied after stop-on-error")
	}
}

func TestCopierPauseAndResumeViaGovernor(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	f1 := filepath.Join(src, "f1.txt")
	f2 := filepath.Join(src, "f2.txt")
	writeFile(t, f1, []byte("one"))
	writeFile(t, f2, []byte("two"))

	q := NewQueue()
	if err := q.Add([]string{f1, f2}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	gov := &scriptGovernor{
		script:       []bool{false, false, true}, // tick@0 block, tick@500ms block, tick@1s allow
		defaultAllow: true,
		pauseState:   StateResting,
	}
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	c := NewCopier(q, dest, gov, clock)
	c.TickInterval = 500 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	// First tick (dt=0) blocks before any write: state comes from the governor.
	waitFor(t, 2*time.Second, func() bool { return c.Snapshot().State == StateResting })
	if _, err := os.Stat(filepath.Join(dest, "f1.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("file must not be copied while the governor blocks")
	}

	clock.set(clock.Now().Add(500 * time.Millisecond)) // tick #2: still blocked
	waitFor(t, 2*time.Second, func() bool { return gov.tickCount() >= 2 })
	if _, err := os.Stat(filepath.Join(dest, "f1.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("file must not be copied while the governor is still blocked")
	}

	clock.set(clock.Now().Add(500 * time.Millisecond)) // tick #3: allow → both files copy
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after the governor allowed copying")
	}

	snap := c.Snapshot()
	if snap.State != StateDone {
		t.Errorf("state = %s, want done", snap.State)
	}
	assertFileContent(t, filepath.Join(dest, "f1.txt"), []byte("one"))
	assertFileContent(t, filepath.Join(dest, "f2.txt"), []byte("two"))

	gov.mu.Lock()
	defer gov.mu.Unlock()
	if len(gov.dts) != 3 {
		t.Fatalf("governor ticks = %d (dts=%v), want exactly 3", len(gov.dts), gov.dts)
	}
	if got := gov.dts[1]; got != 500*time.Millisecond {
		t.Errorf("tick #2 dt = %s, want 500ms", got)
	}
	if got := gov.dts[2]; got != 500*time.Millisecond {
		t.Errorf("tick #3 dt = %s, want 500ms", got)
	}
}

func TestCopierCancelViaContext(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	f1 := filepath.Join(src, "f1.txt")
	f2 := filepath.Join(src, "f2.txt")
	writeFile(t, f1, []byte("one"))
	writeFile(t, f2, []byte("two"))

	q := NewQueue()
	if err := q.Add([]string{f1, f2}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	gov := &scriptGovernor{
		script:       []bool{false}, // blocks forever
		defaultAllow: false,
		pauseState:   StatePausedThermal,
	}
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	c := NewCopier(q, dest, gov, clock)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	waitFor(t, 2*time.Second, func() bool { return c.Snapshot().State == StatePausedThermal })
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	snap := c.Snapshot()
	if snap.State != StateCancelled {
		t.Errorf("state = %s, want cancelled", snap.State)
	}
	items := q.Items()
	for i, it := range items {
		if it.Status != ItemPending {
			t.Errorf("item %d status = %s, want pending (retryable after cancel)", i, it.Status)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "f1.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("no file may be copied before cancellation")
	}
}

func TestCopierProgressSnapshotValues(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	f1 := filepath.Join(src, "f1.txt")
	f2 := filepath.Join(src, "f2.txt")
	writeFile(t, f1, make([]byte, 50))
	writeFile(t, f2, make([]byte, 50))

	q := NewQueue()
	if err := q.Add([]string{f1, f2}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := q.TotalBytes(); got != 100 {
		t.Fatalf("TotalBytes() = %d, want 100", got)
	}

	gov := &scriptGovernor{
		script:       []bool{false, true, false, true}, // block, allow (f1), block (before f2), allow
		defaultAllow: true,
		pauseState:   StateResting,
	}
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	c := NewCopier(q, dest, gov, clock)
	c.TickInterval = 0 // tick at every file boundary → deterministic mid-run freeze

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	// Block #1: resting before any copy.
	waitFor(t, 2*time.Second, func() bool { return c.Snapshot().State == StateResting })
	snap := c.Snapshot()
	if snap.State != StateResting || snap.OverallPercent != 0 {
		t.Errorf("mid-run snapshot = %+v, want state resting percent 0", snap)
	}

	clock.set(clock.Now().Add(500 * time.Millisecond)) // allow → f1 copies; next boundary blocks again
	waitFor(t, 2*time.Second, func() bool {
		return c.Snapshot().State == StateResting && q.Items()[0].Status == ItemDone
	})

	snap = c.Snapshot()
	if snap.State != StateResting {
		t.Errorf("state = %s, want resting (blocked before f2)", snap.State)
	}
	assertClose(t, snap.OverallPercent, 50) // f1 done: 50 of 100 bytes processed
	if snap.CurrentFile != "" {
		t.Errorf("CurrentFile = %q between items, want empty", snap.CurrentFile)
	}

	clock.set(clock.Now().Add(500 * time.Millisecond)) // allow → f2 copies → done
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish")
	}

	snap = c.Snapshot()
	if snap.State != StateDone {
		t.Errorf("state = %s, want done", snap.State)
	}
	assertClose(t, snap.OverallPercent, 100)
	// Elapsed fake time is exactly 1 s and 100 bytes were copied: speed is exact.
	assertClose(t, snap.SpeedBps, 100)
	if snap.ETA != 0 {
		t.Errorf("ETA = %s with nothing left, want 0", snap.ETA)
	}
	assertFileContent(t, filepath.Join(dest, "f2.txt"), make([]byte, 50))
}

func TestCopierRunWithoutDest(t *testing.T) {
	q := NewQueue()
	c := &Copier{Queue: q, Clock: RealClock{}, TickInterval: defaultTickInterval}
	if err := c.Run(context.Background()); err == nil {
		t.Fatal("Run without Dest = nil error, want error")
	}
	if snap := c.Snapshot(); snap.State != StateError {
		t.Errorf("state = %s, want error", snap.State)
	}
}

func TestProgressValues(t *testing.T) {
	pct, speed, eta := progressValues(1000, 250, 250, 5*time.Second)
	assertClose(t, pct, 25)    // 250 of 1000 bytes processed
	assertClose(t, speed, 50)  // 250 B / 5 s
	if eta != 15*time.Second { // (1000-250) remaining at 50 B/s
		t.Errorf("ETA = %s, want 15s", eta)
	}

	pct, speed, eta = progressValues(0, 0, 0, 0)
	if pct != 0 || speed != 0 || eta != 0 {
		t.Errorf("empty queue values = (%v, %v, %v), want all zero", pct, speed, eta)
	}

	pct, speed, eta = progressValues(100, 100, 100, time.Second)
	if pct != 100 || eta != 0 {
		t.Errorf("finished values = (%v, %v, %v), want percent 100 and ETA 0", pct, speed, eta)
	}
}
