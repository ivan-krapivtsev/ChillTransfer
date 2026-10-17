package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fullProfile is a profile with every field set, so round-trips cannot pass by
// accident on zero values.
func fullProfile(id string) Profile {
	return Profile{
		DeviceID:    id,
		Mode:        ModeSensor,
		TPauseC:     55.5,
		TResumeC:    47.25,
		PollSeconds: 7,
		WorkMinutes: 12,
		RestMinutes: 3,
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewStore(path)

	want := []Profile{
		fullProfile("SN-SECOND"),
		fullProfile("1A2B3C4D"),
		{DeviceID: "defaults-only"}, // every optional field left zero
	}
	for _, p := range want {
		store.Set(p)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded := NewStore(path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat after Save: %v", err)
	}
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, p := range want {
		got, ok := reloaded.Get(p.DeviceID)
		if !ok {
			t.Fatalf("Get(%q) after reload: not found", p.DeviceID)
		}
		if got != p {
			t.Errorf("Get(%q) = %+v, want %+v", p.DeviceID, got, p)
		}
	}
	if _, ok := reloaded.Get("never-saved"); ok {
		t.Error(`Get("never-saved") reported a profile`)
	}
}

func TestSaveIsDeterministicAndKeyedByDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewStore(path)
	store.Set(fullProfile("zzz"))
	store.Set(fullProfile("aaa"))
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}

	// Re-saving the same logical state must produce identical bytes and must
	// replace (not append to) the previous document.
	again := NewStore(path)
	if err := again.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := again.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read re-saved file: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("re-save changed the file bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	var docs []Profile
	if err := json.Unmarshal(second, &docs); err != nil {
		t.Fatalf("saved file is not a single JSON document: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("decoded %d profiles, want 2: %s", len(docs), second)
	}
	if docs[0].DeviceID != "aaa" || docs[1].DeviceID != "zzz" {
		t.Errorf("profiles not sorted by device ID: %v", docs)
	}
}

func TestLoadMissingFileIsEmptyStoreNoError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "profiles.json")
	store := NewStore(path)

	if err := store.Load(); err != nil {
		t.Fatalf("Load on missing file: %v, want nil", err)
	}
	if _, ok := store.Get("anything"); ok {
		t.Error("empty store returned a profile")
	}

	// The first Save must create the missing parent directories.
	store.Set(fullProfile("fresh"))
	if err := store.Save(); err != nil {
		t.Fatalf("Save into missing dir: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file created by Save: %v", err)
	}
	if !strings.Contains(string(data), "fresh") {
		t.Errorf("created file does not contain the saved profile: %s", data)
	}
}

func TestLoadCorruptFileReturnsErrorAndKeepsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte("{ this is not json "), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	store := NewStore(path)
	known := fullProfile("keep-me")
	store.Set(known) // in-memory state from before the file went bad

	err := store.Load()
	if err == nil {
		t.Fatal("Load on corrupt JSON returned nil, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the offending file", err)
	}
	got, ok := store.Get("keep-me")
	if !ok || got != known {
		t.Errorf("corrupt Load discarded in-memory profiles: got %+v (found=%v)", got, ok)
	}

	// A valid document must still be rejected when it has the wrong shape.
	if err := os.WriteFile(path, []byte(`{"deviceID":"not-an-array"}`), 0o600); err != nil {
		t.Fatalf("write mistyped file: %v", err)
	}
	if err := store.Load(); err == nil {
		t.Error(`Load on a JSON object instead of an array returned nil, want an error`)
	}
}

func TestSaveReplacesFileInsteadOfTruncating(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	store := NewStore(path)
	long := fullProfile("long-device")
	long.Mode = ModeCycle
	store.Set(long)
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}

	// A much smaller payload would leave trailing bytes behind if the writer
	// opened the target without truncating it.
	small := NewStore(path)
	small.Set(Profile{DeviceID: "s"})
	if err := small.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read re-saved file: %v", err)
	}
	if len(after) >= len(before) {
		t.Errorf("file did not shrink as expected: before %d bytes, after %d bytes", len(before), len(after))
	}
	var docs []Profile
	if err := json.Unmarshal(after, &docs); err != nil {
		t.Fatalf("trailing bytes left in the file: %v", err)
	}
	if len(docs) != 1 || docs[0].DeviceID != "s" {
		t.Errorf("decoded %+v, want a single profile for device s", docs)
	}

	// The staging file must not survive a successful save.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "profiles.json" {
			t.Errorf("leftover file after Save: %s", e.Name())
		}
	}
}

func TestConcurrentGetSetAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewStore(path)

	const writers = 8
	const iterations = 50

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := string(rune('a'+w)) + "-device"
			for i := 0; i < iterations; i++ {
				p := fullProfile(id)
				p.PollSeconds = i
				store.Set(p)

				if got, ok := store.Get(id); !ok || got.DeviceID != id {
					t.Errorf("Get(%q) = %+v (found=%v)", id, got, ok)
					return
				}
				_, _ = store.Get("absent-device")

				if i%10 == 0 {
					if err := store.Save(); err != nil {
						t.Errorf("Save: %v", err)
						return
					}
					if err := store.Load(); err != nil {
						t.Errorf("Load: %v", err)
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()

	for w := 0; w < writers; w++ {
		id := string(rune('a'+w)) + "-device"
		if _, ok := store.Get(id); !ok {
			t.Errorf("profile for %q missing after concurrent writes", id)
		}
	}
	if err := store.Save(); err != nil {
		t.Fatalf("final Save: %v", err)
	}
	check := NewStore(path)
	if err := check.Load(); err != nil {
		t.Fatalf("final Load: %v", err)
	}
	for w := 0; w < writers; w++ {
		id := string(rune('a'+w)) + "-device"
		if _, ok := check.Get(id); !ok {
			t.Errorf("profile for %q missing after reload", id)
		}
	}
}

func TestDefaultPathResolution(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)

	want := filepath.Join(dir, "profiles.json")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}

	// An empty path selects the default location, and saving through it must
	// create the file there.
	store := NewStore("")
	store.Set(fullProfile("default-path-device"))
	if err := store.Save(); err != nil {
		t.Fatalf("Save to default path: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("stat default-path file: %v", err)
	}

	// A nested, not-yet-existing config dir is created on demand.
	nested := filepath.Join(dir, "a", "b")
	t.Setenv(EnvConfigDir, nested)
	nestedStore := NewStore("")
	if err := nestedStore.Save(); err != nil {
		t.Fatalf("Save into %s: %v", nested, err)
	}
	if _, err := os.Stat(filepath.Join(nested, "profiles.json")); err != nil {
		t.Fatalf("stat nested default-path file: %v", err)
	}
}

func TestDefaultPathWithoutEnvUsesUserConfigDir(t *testing.T) {
	t.Setenv(EnvConfigDir, "")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	userDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("os.UserConfigDir unavailable: %v", err)
	}
	want := filepath.Join(userDir, "chilltransfer", "profiles.json")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestSetIgnoresEmptyDeviceID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewStore(path)

	store.Set(fullProfile(""))
	if _, ok := store.Get(""); ok {
		t.Error(`store accepted a profile with an empty device ID`)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := reloaded.Get(""); ok {
		t.Error(`reloaded store reports a profile with an empty device ID`)
	}
}

func TestSetReplacesExistingProfile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	first := fullProfile("dev")
	store.Set(first)

	second := Profile{DeviceID: "dev", Mode: ModeCycle, WorkMinutes: 20}
	store.Set(second)

	got, ok := store.Get("dev")
	if !ok {
		t.Fatal("profile for dev missing after replacement")
	}
	if got != second {
		t.Errorf("Get(dev) = %+v, want %+v", got, second)
	}
}
