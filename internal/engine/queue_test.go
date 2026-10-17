package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestQueueAddFilesAndDirectories(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), make([]byte, 100))
	writeFile(t, filepath.Join(src, "dir", "b.txt"), make([]byte, 200))
	writeFile(t, filepath.Join(src, "dir", "nested", "c.txt"), make([]byte, 300))

	q := NewQueue()
	if err := q.Add([]string{filepath.Join(src, "a.txt"), filepath.Join(src, "dir")}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("Items() = %d items, want 2", len(items))
	}
	if items[0].SizeBytes != 100 || items[0].Status != ItemPending {
		t.Errorf("file item = %+v, want size 100 status pending", items[0])
	}
	if items[1].SizeBytes != 500 { // recursive directory size: 200 + 300
		t.Errorf("dir item size = %d, want 500", items[1].SizeBytes)
	}

	if got := q.TotalBytes(); got != 600 {
		t.Errorf("TotalBytes() = %d, want 600", got)
	}
	if got := q.ProcessedBytes(); got != 0 {
		t.Errorf("ProcessedBytes() = %d, want 0 (all pending)", got)
	}
}

func TestQueueAddMissingPath(t *testing.T) {
	q := NewQueue()
	err := q.Add([]string{filepath.Join(t.TempDir(), "does-not-exist")})
	if err == nil {
		t.Fatal("Add(missing path) = nil error, want error")
	}
	if got := len(q.Items()); got != 0 {
		t.Errorf("failed Add left %d items in queue, want 0", got)
	}
}

func TestQueueRemove(t *testing.T) {
	src := t.TempDir()
	p1 := writeFile2(t, filepath.Join(src, "p1.txt"))
	p2 := writeFile2(t, filepath.Join(src, "p2.txt"))
	p3 := writeFile2(t, filepath.Join(src, "p3.txt"))

	q := NewQueue()
	if err := q.Add([]string{p1, p2, p3}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := q.Remove(1); err != nil {
		t.Fatalf("Remove(1): %v", err)
	}
	items := q.Items()
	if len(items) != 2 || items[0].Path != p1 || items[1].Path != p3 {
		t.Errorf("after Remove(1) = %+v, want [p1 p3]", items)
	}
	if err := q.Remove(-1); err == nil {
		t.Error("Remove(-1): want error")
	}
	if err := q.Remove(99); err == nil {
		t.Error("Remove(99): want error")
	}
}

// writeFile2 writes a 7-byte file and returns its path.
func writeFile2(t *testing.T, path string) string {
	writeFile(t, path, []byte("1234567"))
	return path
}

func TestQueueItemsSnapshotIsIndependent(t *testing.T) {
	src := t.TempDir()
	p1 := writeFile2(t, filepath.Join(src, "p1.txt"))
	p2 := writeFile2(t, filepath.Join(src, "p2.txt"))

	q := NewQueue()
	if err := q.Add([]string{p1, p2}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	snap := q.Items()
	if err := q.Remove(0); err != nil {
		t.Fatalf("Remove(0): %v", err)
	}
	if len(snap) != 2 || snap[1].Status != ItemPending {
		t.Errorf("snapshot after Remove = %+v, want unchanged 2-item snapshot", snap)
	}
	if got := q.TotalBytes(); got != 7 {
		t.Errorf("TotalBytes() after remove = %d, want 7", got)
	}
}

func TestQueueProcessedBytesByStatus(t *testing.T) {
	src := t.TempDir()
	p1 := writeFile2(t, filepath.Join(src, "p1.txt")) // 7 bytes
	p2 := writeFile2(t, filepath.Join(src, "p2.txt")) // 7 bytes
	p3 := writeFile2(t, filepath.Join(src, "p3.txt")) // 7 bytes

	q := NewQueue()
	if err := q.Add([]string{p1, p2, p3}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	cases := []struct {
		status ItemStatus
		want   uint64
	}{
		{ItemPending, 0},
		{ItemCopying, 0}, // in-flight bytes are not processed yet
		{ItemDone, 7},
		{ItemSkipped, 7},
		{ItemFailed, 7},
	}
	for _, tc := range cases {
		q.setStatus(1, tc.status)
		if got := q.ProcessedBytes(); got != tc.want {
			t.Errorf("ProcessedBytes() with item status %s = %d, want %d", tc.status.String(), got, tc.want)
		}
	}

	q.setStatus(0, ItemDone)
	q.setStatus(2, ItemFailed)
	if got := q.ProcessedBytes(); got != 21 {
		t.Errorf("ProcessedBytes() all terminal = %d, want 21", got)
	}
}

func TestItemStatusStrings(t *testing.T) {
	cases := map[ItemStatus]string{
		ItemPending: "pending", ItemCopying: "copying", ItemDone: "done",
		ItemFailed: "failed", ItemSkipped: "skipped",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("ItemStatus(%d).String() = %q, want %q", int(status), got, want)
		}
	}
}
