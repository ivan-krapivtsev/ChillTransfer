// Package profiles persists per-device settings (SPEC FR-6): the copy mode and
// the thresholds/timings chosen for one device are stored on disk and restored
// when the same device is selected again.
//
// Storage is a single JSON file holding an array of Profile objects keyed by
// DeviceID (the stable device identifier from internal/devices). The default
// location is platform-appropriate, resolved by os.UserConfigDir:
//
//	macOS   ~/Library/Application Support/chilltransfer/profiles.json
//	Linux   ~/.config/chilltransfer/profiles.json  (or $XDG_CONFIG_HOME)
//	Windows %AppData%\chilltransfer\profiles.json
//
// The environment variable CHILLTRANSFER_CONFIG_DIR replaces the config
// directory half of that path when set, which keeps the default location
// testable and lets a user point the app at a custom directory.
//
// A Store owns the in-memory copy of that file and is safe for concurrent use:
// the UI reads profiles from its goroutine while saving happens elsewhere.
package profiles
