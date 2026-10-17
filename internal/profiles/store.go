package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// EnvConfigDir overrides the config directory used for the default
// profiles.json location. Only the directory is replaced; the file name is
// fixed. See the package documentation for the resolved defaults.
const EnvConfigDir = "CHILLTRANSFER_CONFIG_DIR"

// dirPerm and filePerm keep user settings private to the account: the file is
// created by os.CreateTemp, which already uses 0600.
const dirPerm fs.FileMode = 0o700

// fileName is the profile file name inside the config directory.
const fileName = "profiles.json"

// tmpPattern is the os.CreateTemp pattern for the staging file written before
// it is renamed onto fileName. The leading dot keeps it out of casual listings.
const tmpPattern = ".profiles-*.json.tmp"

// DefaultPath returns the profiles.json location used when Store is created
// with an empty path: $CHILLTRANSFER_CONFIG_DIR/profiles.json when that
// variable is set, otherwise <os.UserConfigDir>/chilltransfer/profiles.json.
func DefaultPath() (string, error) {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return filepath.Join(dir, fileName), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("profiles: resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "chilltransfer", fileName), nil
}

// Store keeps the per-device profiles of one profiles.json file in memory and
// persists them atomically. The zero value is not usable; call NewStore.
//
// Concurrency: a mutex guards the in-memory map, so Get/Set/Load/Save may be
// called from any goroutine. File I/O happens under the same lock, which keeps
// a Save from writing a snapshot that no Set ever produced.
type Store struct {
	mu   sync.RWMutex
	path string
	// initErr records an unresolvable default path so the failure surfaces on
	// the first Load/Save instead of being swallowed by NewStore's signature.
	initErr error
	byID    map[string]Profile
}

// NewStore returns an empty store backed by path. An empty path selects
// DefaultPath; if that cannot be resolved, Load and Save report the resolution
// error. Parent directories are created by Save, not here.
func NewStore(path string) *Store {
	s := &Store{path: path, byID: map[string]Profile{}}
	if path == "" {
		def, err := DefaultPath()
		if err != nil {
			s.initErr = err
		} else {
			s.path = def
		}
	}
	return s
}

// Load replaces the in-memory profiles with the contents of the file.
//
// A missing file is not an error: the store becomes empty and the first Save
// creates the file. A corrupt or unreadable file returns an error and leaves
// the in-memory profiles untouched, so a damaged settings file can never
// silently discard the user's saved configuration.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initErr != nil {
		return s.initErr
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.byID = map[string]Profile{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("profiles: read %s: %w", s.path, err)
	}

	var loaded []Profile
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("profiles: parse %s: %w", s.path, err)
	}

	byID := make(map[string]Profile, len(loaded))
	for _, p := range loaded {
		if p.DeviceID == "" {
			continue // entry without a key can never be restored
		}
		byID[p.DeviceID] = p
	}
	s.byID = byID
	return nil
}

// Save writes the in-memory profiles to the file atomically: the payload goes
// to a temporary file in the same directory, which is then renamed onto the
// target. Readers therefore see either the previous complete file or the new
// one, never a truncated or half-written version. Missing parent directories
// are created first. Entries are written sorted by DeviceID so repeated saves
// of unchanged data produce identical bytes.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initErr != nil {
		return s.initErr
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("profiles: create dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s.sorted(), "", "  ")
	if err != nil {
		return fmt.Errorf("profiles: encode %d profile(s): %w", len(s.byID), err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return fmt.Errorf("profiles: create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	replaced := false
	defer func() {
		if !replaced {
			// Best effort: a leftover staging file must not mask the real error.
			_ = os.Remove(tmpName)
		}
	}()

	if err := writeAndSync(tmp, data); err != nil {
		return fmt.Errorf("profiles: write %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("profiles: replace %s: %w", s.path, err)
	}
	replaced = true
	return nil
}

// writeAndSync writes data to f, flushes it to stable storage and closes f.
// The explicit Sync is deliberate: this file holds the user's settings and the
// project treats durability of small config writes as a requirement.
func writeAndSync(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Get returns the stored profile for deviceID. The second result is false when
// no profile was saved for that ID, in which case the caller applies defaults.
func (s *Store) Get(deviceID string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[deviceID]
	return p, ok
}

// Set stores p under p.DeviceID, replacing any previous profile for that
// device. A Profile with an empty DeviceID is ignored: it could never be
// restored by ID.
func (s *Store) Set(p Profile) {
	if p.DeviceID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.byID[p.DeviceID] = p
}

// sorted returns the profiles ordered by DeviceID for a deterministic file.
// Callers must hold s.mu.
func (s *Store) sorted() []Profile {
	out := make([]Profile, 0, len(s.byID))
	for _, p := range s.byID {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Profile) int { return strings.Compare(a.DeviceID, b.DeviceID) })
	return out
}
