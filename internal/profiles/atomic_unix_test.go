//go:build unix

package profiles

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// inode returns the file serial number of path on unix-like systems.
func inode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skipf("no syscall.Stat_t for %s", path)
	}
	return stat.Ino
}

// TestSaveRenamesOverTarget is the strong atomicity check: a save must install
// a brand-new inode rather than truncating the existing file in place, so a
// reader that already opened the old version keeps seeing complete previous
// content while the path serves the new one.
func TestSaveRenamesOverTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")

	store := NewStore(path)
	store.Set(fullProfile("first"))
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := inode(t, path)

	// Keep a handle open on the first version, as a concurrent reader would.
	reader, err := os.Open(path)
	if err != nil {
		t.Fatalf("open saved file: %v", err)
	}
	defer reader.Close()

	store.Set(fullProfile("second"))
	if err := store.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	if after := inode(t, path); after == before {
		t.Errorf("Save reused inode %d: target was truncated in place, not replaced by rename", before)
	}

	stale, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read through the pre-save handle: %v", err)
	}
	if !savedHasDevice(t, stale, "first") || savedHasDevice(t, stale, "second") {
		t.Errorf("the previously opened file no longer holds its original content: %s", stale)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current file: %v", err)
	}
	if !savedHasDevice(t, current, "second") {
		t.Errorf("current file does not hold the new profile: %s", current)
	}
}

// savedHasDevice reports whether a saved document contains deviceID.
func savedHasDevice(t *testing.T, data []byte, deviceID string) bool {
	t.Helper()
	var docs []Profile
	if err := json.Unmarshal(data, &docs); err != nil {
		t.Fatalf("decode saved document %q: %v", data, err)
	}
	for _, d := range docs {
		if d.DeviceID == deviceID {
			return true
		}
	}
	return false
}
