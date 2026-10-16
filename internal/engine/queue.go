package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// ItemStatus is a status of a queue item during copying.
type ItemStatus int

const (
	ItemPending ItemStatus = iota // not started yet
	ItemCopying                   // currently being copied
	ItemDone                      // successfully copied
	ItemFailed                    // copy failed with an error
	ItemSkipped                   // skipped: existing file matched size+mtime
)

func (s ItemStatus) String() string {
	switch s {
	case ItemPending:
		return "pending"
	case ItemCopying:
		return "copying"
	case ItemDone:
		return "done"
	case ItemFailed:
		return "failed"
	case ItemSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// terminal reports whether the item status is final (no longer pending/copying).
func (s ItemStatus) terminal() bool {
	switch s {
	case ItemDone, ItemFailed, ItemSkipped:
		return true
	default:
		return false
	}
}

// Item is a single entry in the copy queue: one file or directory.
type Item struct {
	Path      string     // source path (file or directory)
	SizeBytes uint64     // size in bytes; for directories — recursive sum of file sizes
	Status    ItemStatus // current status
}

// Queue holds items to be copied. All methods are safe for concurrent use:
// the copier mutates item statuses from its own goroutine while the UI reads
// snapshots (architecture decision: no data races on engine state).
type Queue struct {
	mu    sync.Mutex
	items []Item
}

func NewQueue() *Queue { return &Queue{} }

// Add appends files and directories to the queue, computing directory sizes
// recursively. All paths must exist; otherwise an error is returned and no
// items from this call are added.
func (q *Queue) Add(paths []string) error {
	added := make([]Item, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("engine: add %s: %w", p, err)
		}
		var size uint64
		switch {
		case info.IsDir():
			size, err = dirSize(p)
			if err != nil {
				return fmt.Errorf("engine: size of %s: %w", p, err)
			}
		case info.Mode().IsRegular():
			size = uint64(info.Size())
		default:
			return fmt.Errorf("engine: add %s: unsupported file type %v", p, info.Mode())
		}
		added = append(added, Item{Path: p, SizeBytes: size})
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range added {
		q.items = append(q.items, it)
	}
	return nil
}

// Remove deletes the item at index. Intended for use before copying starts;
// removing items while a run is in progress shifts indices and is unsupported.
func (q *Queue) Remove(index int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index >= len(q.items) {
		return fmt.Errorf("engine: remove %d: out of range", index)
	}
	q.items = append(q.items[:index], q.items[index+1:]...)
	return nil
}

// Items returns a snapshot copy of the queue; later mutations do not affect it.
func (q *Queue) Items() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	copy(out, q.items)
	return out
}

// TotalBytes is the sum of sizes of all items currently in the queue.
func (q *Queue) TotalBytes() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var total uint64
	for _, it := range q.items {
		total += it.SizeBytes
	}
	return total
}

// ProcessedBytes is the sum of sizes of items with a terminal status
// (done, failed or skipped): work on them will not be repeated.
func (q *Queue) ProcessedBytes() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var processed uint64
	for _, it := range q.items {
		if it.Status.terminal() {
			processed += it.SizeBytes
		}
	}
	return processed
}

// itemAt returns the item at index, or a zero Item when out of range.
// Internal: used by the copier (same package).
func (q *Queue) itemAt(index int) Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index >= len(q.items) {
		return Item{}
	}
	return q.items[index]
}

// setStatus updates the status of the item at index. Internal: used by the
// copier (same package). Out-of-range indices are ignored.
func (q *Queue) setStatus(index int, s ItemStatus) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index >= 0 && index < len(q.items) {
		q.items[index].Status = s
	}
}

// dirSize computes the recursive size of a directory in bytes (regular files;
// symlinks are counted by their own link size, not followed).
func dirSize(root string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += uint64(info.Size())
		return nil
	})
	return total, err
}
