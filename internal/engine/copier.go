package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// defaultTickInterval is the governor polling period used by NewCopier.
const defaultTickInterval = 500 * time.Millisecond

// copyBufferBytes is the file-copy buffer size (~1 MB).
const copyBufferBytes = 1 << 20

// pollInterval is how often Run yields real time while waiting for a blocked
// governor or context cancellation (independent of Clock, so fake clocks can
// be advanced from tests).
const pollInterval = 20 * time.Millisecond

// Copier copies queue items to Dest with thermal-aware pauses. Construct via
// NewCopier for defaults. Run must not be called concurrently; a second call
// after a terminal state re-processes the items that are still pending.
type Copier struct {
	Queue        *Queue
	Governor     Governor // nil means no pacing: copying is always allowed
	Clock        Clock
	Dest         string        // destination root directory (required)
	Overwrite    bool          // false (default): skip existing files whose size+mtime match
	StopOnError  bool          // true: stop the whole run on the first file error
	TickInterval time.Duration // governor polling period; 0 means check at every file boundary

	mu          sync.Mutex
	state       State
	currentFile string
	copiedBytes uint64
	runStart    time.Time

	// The fields below are only accessed from the Run goroutine.
	lastTick time.Time
	nextTick time.Time
	buf      []byte
}

// NewCopier returns a Copier with default values: 500 ms TickInterval and a
// real clock when clock is nil. Governor may be nil (no pacing).
func NewCopier(queue *Queue, dest string, gov Governor, clock Clock) *Copier {
	if queue == nil {
		queue = NewQueue()
	}
	if clock == nil {
		clock = RealClock{}
	}
	return &Copier{
		Queue:        queue,
		Governor:     gov,
		Clock:        clock,
		Dest:         dest,
		TickInterval: defaultTickInterval,
	}
}

// Snapshot is a point-in-time view of the copy process for UI polling.
type Snapshot struct {
	State          State
	OverallPercent float64       // 0..100 over all queue bytes; done/failed/skipped count as processed
	CurrentFile    string        // source path currently being written; empty between items and when idle
	SpeedBps       float64       // average throughput since run start, including pauses
	ETA            time.Duration // remaining bytes / SpeedBps; zero when unknown or nothing left
}

// Snapshot returns a thread-safe point-in-time view of the process.
func (c *Copier) Snapshot() Snapshot {
	c.mu.Lock()
	state := c.state
	currentFile := c.currentFile
	copiedBytes := c.copiedBytes
	runStart := c.runStart
	c.mu.Unlock()

	total := c.Queue.TotalBytes()
	processed := c.Queue.ProcessedBytes()

	var elapsed time.Duration
	if !runStart.IsZero() {
		elapsed = c.Clock.Now().Sub(runStart)
	}
	pct, speed, eta := progressValues(total, processed, copiedBytes, elapsed)
	return Snapshot{State: state, OverallPercent: pct, CurrentFile: currentFile, SpeedBps: speed, ETA: eta}
}

// Run processes all pending queue items until completion, a file error with
// StopOnError set, or context cancellation. It blocks (without writing) while
// the governor reports copying blocked. Return value: nil on done/cancelled;
// the file error when StopOnError stopped the run.
func (c *Copier) Run(ctx context.Context) error {
	if c.Queue == nil || c.Clock == nil {
		return errors.New("engine: copier requires Queue and Clock")
	}
	if c.Dest == "" {
		c.setState(StateError)
		return errors.New("engine: copier destination is not set")
	}
	if err := os.MkdirAll(c.Dest, 0o755); err != nil {
		c.setState(StateError)
		return fmt.Errorf("engine: create destination %s: %w", c.Dest, err)
	}

	now := c.Clock.Now()
	c.mu.Lock()
	c.state = StateCopying
	c.currentFile = ""
	c.copiedBytes = 0
	c.runStart = now
	c.lastTick = now
	c.nextTick = now // first governor evaluation is immediate (dt=0)
	if c.buf == nil {
		c.buf = make([]byte, copyBufferBytes)
	}
	c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		c.finishRun(StateCancelled)
		return nil
	}

	count := len(c.Queue.Items()) // queue is stable during a run (removals happen before start)
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			c.finishRun(StateCancelled)
			return nil
		}
		item := c.Queue.itemAt(i)
		if item.Path == "" || item.Status != ItemPending {
			continue // already processed in a previous Run call
		}

		c.Queue.setStatus(i, ItemCopying)
		status, err := c.processItem(ctx, i)
		if ctx.Err() != nil {
			c.Queue.setStatus(i, ItemPending) // cancelled mid-item: keep it retryable
			c.finishRun(StateCancelled)
			return nil
		}
		c.Queue.setStatus(i, status)
		c.setCurrentFile("") // between items: no file is being written
		if err != nil {
			if c.StopOnError {
				c.finishRun(StateError)
				return err
			}
			continue // item marked failed; continue with the next one (FR-3)
		}
	}

	c.finishRun(StateDone)
	return nil
}

// processItem copies a single queue item (file or directory). It does not
// change the item status itself; Run applies the returned status. On context
// cancellation it returns ItemPending so the item can be retried on the next
// run. Directory structure and file mtimes are preserved (FR-3); directory
// mtimes are restored best-effort after their children are copied.
func (c *Copier) processItem(ctx context.Context, index int) (ItemStatus, error) {
	item := c.Queue.itemAt(index)

	info, err := os.Stat(item.Path)
	if err != nil {
		return ItemFailed, fmt.Errorf("engine: stat %s: %w", item.Path, err)
	}

	if err := c.ensureAllowed(ctx); err != nil {
		return ItemPending, err
	}
	c.setCurrentFile(item.Path)

	if !info.IsDir() {
		skipped, cerr := c.copyFile(item.Path, c.destOf(item.Path, item.Path))
		if cerr != nil {
			return ItemFailed, cerr
		}
		if skipped {
			return ItemSkipped, nil
		}
		return ItemDone, nil
	}

	var nCopied, nSkipped int
	type dirTime struct {
		src, dest string
		mod       time.Time
	}
	var dirs []dirTime
	walkErr := filepath.WalkDir(item.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		dest := c.destOf(item.Path, path)
		if d.IsDir() {
			di, ierr := d.Info()
			if ierr == nil {
				dirs = append(dirs, dirTime{src: path, dest: dest, mod: di.ModTime()})
			}
			return os.MkdirAll(dest, 0o755)
		}
		if aerr := c.ensureAllowed(ctx); aerr != nil {
			return aerr
		}
		c.setCurrentFile(path)
		skipped, ferr := c.copyFile(path, dest)
		if ferr != nil {
			return ferr
		}
		if skipped {
			nSkipped++
		} else {
			nCopied++
		}
		return nil
	})
	if walkErr != nil {
		if ctx.Err() != nil || errors.Is(walkErr, context.Canceled) {
			return ItemPending, context.Canceled
		}
		return ItemFailed, walkErr
	}

	// Directory mtimes: set after children exist so they are not clobbered.
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Chtimes(dirs[i].dest, dirs[i].mod, dirs[i].mod) // best-effort (FR-3)
	}

	if nCopied == 0 && nSkipped > 0 {
		return ItemSkipped, nil
	}
	return ItemDone, nil
}

// copyFile copies src to dest preserving mode and mtime. When !Overwrite and
// an existing regular file at dest matches size+mtime it is skipped (FR-3).
// Returns true when the file was skipped. The destination file is fsynced
// before close so data survives a sudden unplug (FR-3).
func (c *Copier) copyFile(src, dest string) (bool, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false, fmt.Errorf("engine: stat %s: %w", src, err)
	}

	if !c.Overwrite {
		dstInfo, derr := os.Lstat(dest)
		switch {
		case derr == nil && dstInfo.IsDir():
			return false, fmt.Errorf("engine: destination %s is a directory", dest)
		case derr == nil:
			if dstInfo.Size() == srcInfo.Size() && dstInfo.ModTime().Equal(srcInfo.ModTime()) {
				return true, nil // size+mtime match: skip (FR-3)
			}
		case !errors.Is(derr, fs.ErrNotExist):
			return false, fmt.Errorf("engine: stat %s: %w", dest, derr)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, fmt.Errorf("engine: mkdir for %s: %w", dest, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return false, fmt.Errorf("engine: open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return false, fmt.Errorf("engine: create %s: %w", dest, err)
	}
	_ = out.Chmod(srcInfo.Mode().Perm()) // exact mode regardless of umask (best-effort)

	n, cerr := io.CopyBuffer(out, in, c.buf)
	if cerr == nil {
		cerr = out.Sync() // fsync before close (FR-3)
	}
	if e := out.Close(); cerr == nil {
		cerr = e
	}
	if cerr != nil {
		return false, fmt.Errorf("engine: copy %s -> %s: %w", src, dest, cerr)
	}

	c.addCopiedBytes(uint64(n))
	_ = os.Chtimes(dest, srcInfo.ModTime(), srcInfo.ModTime()) // best-effort (FR-3)
	return false, nil
}

// ensureAllowed ticks the governor when due and waits (yielding real time so
// fake clocks can advance in tests) while it reports copying blocked. The
// first evaluation happens immediately with dt=0, so an already-hot device
// pauses before any write. Returns ctx.Err() if cancelled during the wait.
func (c *Copier) ensureAllowed(ctx context.Context) error {
	if c.Governor == nil {
		return nil
	}
	for {
		now := c.Clock.Now()
		if now.Before(c.nextTick) {
			return nil // not due yet; the previous allow stands
		}
		dt := now.Sub(c.lastTick)
		if dt < 0 {
			dt = 0 // clock went backwards (e.g. rewound in tests)
		}
		c.lastTick = now
		c.nextTick = now.Add(c.tickInterval())
		allow, _ := c.Governor.Tick(dt)
		if allow {
			c.setState(StateCopying)
			return nil
		}
		c.setState(c.Governor.PauseState())
		time.Sleep(pollInterval) // yield so a fake clock can advance; paces re-ticks
		for c.Clock.Now().Before(c.nextTick) {
			if err := ctx.Err(); err != nil {
				return err
			}
			time.Sleep(pollInterval)
		}
	}
}

// destOf maps a source path inside queue item root to its destination under
// Dest, preserving the structure below the item's base name.
func (c *Copier) destOf(root, src string) string {
	if src == root {
		return filepath.Join(c.Dest, filepath.Base(root))
	}
	rel, err := filepath.Rel(root, src)
	if err != nil {
		return filepath.Join(c.Dest, filepath.Base(root), filepath.Base(src))
	}
	return filepath.Join(c.Dest, filepath.Base(root), rel)
}

// progressValues computes overall percent, average speed and ETA. Extracted as
// a pure function for deterministic unit testing.
func progressValues(total, processed, copiedBytes uint64, elapsed time.Duration) (percent float64, speed float64, eta time.Duration) {
	if total > 0 {
		percent = float64(processed) / float64(total) * 100
	}
	if elapsed > 0 {
		speed = float64(copiedBytes) / elapsed.Seconds()
	}
	if speed > 0 && processed < total {
		eta = time.Duration(float64(total-processed) / speed * float64(time.Second))
	}
	return percent, speed, eta
}

func (c *Copier) tickInterval() time.Duration {
	if c.TickInterval >= 0 {
		return c.TickInterval
	}
	return defaultTickInterval
}

func (c *Copier) setState(s State) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
}

func (c *Copier) setCurrentFile(path string) {
	c.mu.Lock()
	c.currentFile = path
	c.mu.Unlock()
}

func (c *Copier) addCopiedBytes(n uint64) {
	c.mu.Lock()
	c.copiedBytes += n
	c.mu.Unlock()
}

// finishRun sets the terminal state and clears the current file.
func (c *Copier) finishRun(s State) {
	c.mu.Lock()
	c.state = s
	c.currentFile = ""
	c.mu.Unlock()
}
