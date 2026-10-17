package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/ivan-krapivtsev/chilltransfer/internal/devices"
	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

// fakeLister is an in-memory DeviceLister for headless tests.
type fakeLister struct {
	devices []devices.Device
	err     error
}

func (f *fakeLister) List(bool) ([]devices.Device, error) { return f.devices, f.err }

// fixedTemp is a deterministic engine.TemperatureSource.
type fixedTemp float64

func (f fixedTemp) Read() (float64, bool) { return float64(f), true }

// newTestUI builds a UI with Russian pinned (T-007): widget text assertions in
// this package are written against the RU table regardless of host locale.
func newTestUI(t *testing.T, lister ui.DeviceLister) *ui.UI {
	t.Helper()
	return ui.NewWithI18N(test.NewApp(), lister, ui.NewI18N(ui.LangRU))
}

// mountedDevice returns a device whose MountPoint is a fresh temp dir; withSensor
// attaches a permanently hot (80 °C ≥ default T_pause 50 °C) sensor source.
func mountedDevice(t *testing.T, withSensor bool) devices.Device {
	t.Helper()
	dev := devices.Device{ID: "usb-1", Label: "Test USB", Removable: true, MountPoint: t.TempDir()}
	if withSensor {
		dev.TempSource = fixedTemp(80)
	}
	return dev
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fileURI(t *testing.T, path string) fyne.URI {
	t.Helper()
	u, err := storage.ParseURI("file://" + path)
	if err != nil {
		t.Fatalf("parse URI for %s: %v", path, err)
	}
	return u
}

// waitFor polls the UI until cond holds or the deadline passes. Tests run with
// PollInterval == 0 (no background ticker), so this is the only widget writer.
func waitFor(t *testing.T, u *ui.UI, what string, cond func() bool) {
	t.Helper()
	// 30 s deadline: CI runners are slow and the Run goroutine may be
	// starved under parallel test load; the state transition itself is fast.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		u.Poll()
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s (state label: %q)", what, u.StateLabel.Text)
}

// TestDeviceListRendersFromInjectedProvider verifies the device dropdown is built
// from the injected provider: capacity/free/file system in options, unmounted
// devices marked and excluded as targets, first device auto-selected.
func TestDeviceListRendersFromInjectedProvider(t *testing.T) {
	l := &fakeLister{devices: []devices.Device{
		{ID: "usb-1", Label: "SanDisk 32GB", CapacityBytes: 32 << 30, FreeBytes: 20 << 30, FileSystem: "exFAT", Removable: true, MountPoint: "/Volumes/SanDisk"},
		{ID: "usb-2", Label: "WD Elements", CapacityBytes: 1 << 40, FreeBytes: 512 << 30, FileSystem: "NTFS", Removable: true}, // unmounted
	}}
	u := newTestUI(t, l)

	if len(u.DeviceSelect.Options) != 2 {
		t.Fatalf("expected 2 device options, got %d: %v", len(u.DeviceSelect.Options), u.DeviceSelect.Options)
	}
	if !strings.Contains(u.DeviceSelect.Options[0], "SanDisk 32GB") || !strings.Contains(u.DeviceSelect.Options[0], "exFAT") {
		t.Errorf("option 0 missing device info: %q", u.DeviceSelect.Options[0])
	}
	if !strings.HasSuffix(u.DeviceSelect.Options[1], "(не смонтировано)") {
		t.Errorf("unmounted device not marked: %q", u.DeviceSelect.Options[1])
	}

	dev := u.SelectedDevice()
	if dev == nil || dev.ID != "usb-1" {
		t.Fatalf("expected usb-1 auto-selected, got %+v", dev)
	}
	if u.SensorLabel.Text != "Датчик: не обнаружен" {
		t.Errorf("sensor label = %q, want «Датчик: не обнаружен»", u.SensorLabel.Text)
	}
	if u.ModeRadio.Selected != u.I18N.T(ui.KeyModeCycle) {
		t.Errorf("mode = %q, want Цикл (no sensor)", u.ModeRadio.Selected)
	}
}

// TestDeviceWithSensorAutoSelectsSensorMode verifies auto mode selection from
// device.TempSource and that a manual radio override sticks.
func TestDeviceWithSensorAutoSelectsSensorMode(t *testing.T) {
	u := newTestUI(t, &fakeLister{devices: []devices.Device{
		{ID: "usb-1", Label: "USB", MountPoint: "/Volumes/USB", TempSource: fixedTemp(45)},
	}})

	if u.SensorLabel.Text != "Датчик: обнаружен" {
		t.Errorf("sensor label = %q, want «Датчик: обнаружен»", u.SensorLabel.Text)
	}
	if u.ModeRadio.Selected != u.I18N.T(ui.KeyModeSensor) {
		t.Errorf("mode = %q, want Датчик (sensor present)", u.ModeRadio.Selected)
	}

	u.ModeRadio.SetSelected(u.I18N.T(ui.KeyModeCycle)) // manual override allowed
	if u.ModeRadio.Selected != u.I18N.T(ui.KeyModeCycle) {
		t.Errorf("manual mode override lost: %q", u.ModeRadio.Selected)
	}
}

// TestRefreshDevicesPreservesSelection verifies "Обновить" re-enumerates via the
// provider and keeps the selection by device ID even when option text changes.
func TestRefreshDevicesPreservesSelection(t *testing.T) {
	l := &fakeLister{devices: []devices.Device{
		{ID: "usb-1", Label: "SanDisk 32GB", CapacityBytes: 32 << 30, FreeBytes: 20 << 30, FileSystem: "exFAT", Removable: true, MountPoint: "/Volumes/SanDisk"},
		{ID: "usb-2", Label: "WD Elements", CapacityBytes: 1 << 40, FreeBytes: 512 << 30, FileSystem: "NTFS", Removable: true},
	}}
	u := newTestUI(t, l)
	if got := u.SelectedDevice(); got == nil || got.ID != "usb-1" {
		t.Fatalf("initial selection = %+v, want usb-1", got)
	}

	// Re-enumeration: usb-1 free space changed (option text changes), usb-2 gone.
	l.devices = []devices.Device{
		{ID: "usb-1", Label: "SanDisk 32GB", CapacityBytes: 32 << 30, FreeBytes: 5 << 30, FileSystem: "exFAT", Removable: true, MountPoint: "/Volumes/SanDisk"},
	}
	u.RefreshButton.OnTapped()

	if got := u.SelectedDevice(); got == nil || got.ID != "usb-1" {
		t.Fatalf("selection not preserved by ID after refresh: %+v", got)
	}
	if len(u.DeviceSelect.Options) != 1 {
		t.Errorf("options after refresh = %d, want 1", len(u.DeviceSelect.Options))
	}
}

// TestDroppedURIsLandInQueue verifies dropped file URIs are added to the queue
// with correct sizes and reflected in the queue list.
func TestDroppedURIsLandInQueue(t *testing.T) {
	u := newTestUI(t, &fakeLister{})
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	writeFile(t, f1, 1024)
	f2 := filepath.Join(dir, "b.bin")
	writeFile(t, f2, 2048)

	u.HandleDrop([]fyne.URI{fileURI(t, f1), fileURI(t, f2)})

	items := u.Queue.Items()
	if len(items) != 2 {
		t.Fatalf("queue length = %d, want 2", len(items))
	}
	// fyne URI.Path() normalizes separators (forward slashes on Windows), so
	// compare Clean-normalized paths instead of raw string equality.
	if filepath.Clean(items[0].Path) != filepath.Clean(f1) || items[0].SizeBytes != 1024 {
		t.Errorf("item 0 = %+v, want path=%s size=1024", items[0], f1)
	}
	if filepath.Clean(items[1].Path) != filepath.Clean(f2) || items[1].SizeBytes != 2048 {
		t.Errorf("item 1 = %+v, want path=%s size=2048", items[1], f2)
	}
	if got := u.QueueList.Length(); got != 2 {
		t.Errorf("queue list length = %d, want 2", got)
	}
}

// TestStartRejectsInvalidModeInputs verifies out-of-range cycle parameters block
// start with an error label, while SPEC §5 defaults are accepted.
func TestStartRejectsInvalidModeInputs(t *testing.T) {
	u := newTestUI(t, &fakeLister{devices: []devices.Device{mountedDevice(t, false)}})
	u.PollInterval = 0 // drive Poll() manually from this goroutine

	u.WorkEntry.SetText("0") // out of range 1..60
	if err := u.Start(); err == nil {
		t.Fatal("Start accepted out-of-range work duration")
	}
	if u.ErrorLabel.Text == "" {
		t.Error("error label not set after failed start")
	}
	if u.Running() {
		t.Error("run must not start on invalid inputs")
	}

	u.WorkEntry.SetText("10") // SPEC §5 defaults are valid again
	u.RestEntry.SetText("5")
	if err := u.Start(); err != nil {
		t.Fatalf("Start with default parameters: %v", err)
	}
	// Empty queue completes immediately; wait for the terminal state.
	waitFor(t, u, "done state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStateDone))
	})
}

// TestStartSensorModeWithoutSensorRejected verifies sensor mode cannot start on a
// device without a temperature source (SPEC §4 fallback rule).
// TestStartSensorModeRejectsOutOfRangeTemps verifies sensor-mode inputs are
// validated on Start (SPEC §5): out-of-range T_pause, T_resume >= T_pause and a
// zero poll interval block start, while SPEC §5 defaults start a run that
// thermally pauses on a hot sensor.
func TestStartSensorModeRejectsOutOfRangeTemps(t *testing.T) {
	srcDir := t.TempDir()
	writeFile(t, filepath.Join(srcDir, "data.bin"), 4096)

	u := newTestUI(t, &fakeLister{devices: []devices.Device{mountedDevice(t, true)}})
	u.PollInterval = 0 // drive Poll() manually from this goroutine
	u.ModeRadio.SetSelected(u.I18N.T(ui.KeyModeSensor))
	u.HandleDrop([]fyne.URI{fileURI(t, filepath.Join(srcDir, "data.bin"))})

	cases := []struct {
		name     string
		tPause   string
		tResume  string
		pollSecs string
	}{
		{"tpause above range", "90", "42", "3"},
		{"tresume equals tpause", "50", "50", "3"},
		{"poll zero", "50", "42", "0"},
	}
	for _, tc := range cases {
		u.TPauseEntry.SetText(tc.tPause)
		u.TResumeEntry.SetText(tc.tResume)
		u.SensorPollEntry.SetText(tc.pollSecs)
		if err := u.Start(); err == nil {
			t.Fatalf("%s: Start accepted invalid sensor parameters", tc.name)
		}
		if u.ErrorLabel.Text == "" {
			t.Errorf("%s: error label not set after failed start", tc.name)
		}
		if u.Running() {
			t.Fatalf("%s: run must not start on invalid inputs", tc.name)
		}
	}

	// SPEC §5 defaults are accepted; the hot sensor (80 °C ≥ 50 °C) pauses the run.
	u.TPauseEntry.SetText("50")
	u.TResumeEntry.SetText("42")
	u.SensorPollEntry.SetText("3")
	if err := u.Start(); err != nil {
		t.Fatalf("Start with default sensor parameters: %v", err)
	}
	defer u.Stop()

	waitFor(t, u, "paused_thermal state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStatePausedThermal))
	})
	if !u.Running() {
		t.Fatal("run must stay active while thermally paused")
	}

	u.Stop()
	waitFor(t, u, "cancelled state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStateCancelled))
	})
}

func TestStartSensorModeWithoutSensorRejected(t *testing.T) {
	u := newTestUI(t, &fakeLister{devices: []devices.Device{mountedDevice(t, false)}})
	u.ModeRadio.SetSelected(u.I18N.T(ui.KeyModeSensor))

	if err := u.Start(); err == nil {
		t.Fatal("Start accepted sensor mode without a temperature source")
	}
	if !strings.Contains(u.ErrorLabel.Text, "датчик не обнаружен") {
		t.Errorf("error label = %q, want mention of missing sensor", u.ErrorLabel.Text)
	}
}

// TestStartStopTransitionsState verifies start/stop wiring: a hot sensor blocks
// the copier (paused_thermal), Stop cancels the run context and the state ends
// cancelled.
func TestStartStopTransitionsState(t *testing.T) {
	srcDir := t.TempDir()
	writeFile(t, filepath.Join(srcDir, "data.bin"), 4096)

	u := newTestUI(t, &fakeLister{devices: []devices.Device{mountedDevice(t, true)}})
	u.PollInterval = 0 // drive Poll() manually from this goroutine
	u.HandleDrop([]fyne.URI{fileURI(t, filepath.Join(srcDir, "data.bin"))})

	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer u.Stop()

	// Hot sensor (80 °C ≥ T_pause 50 °C) blocks the copier before copying.
	waitFor(t, u, "paused_thermal state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStatePausedThermal))
	})
	if !u.Running() {
		t.Fatal("run must stay active while thermally paused")
	}

	u.Stop()
	waitFor(t, u, "cancelled state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStateCancelled))
	})
	if u.Running() {
		t.Fatal("run must be inactive after Stop")
	}
}

// TestStartCompletesSmallCopy verifies the happy path: cycle mode copies a small
// file to the destination volume and ends done with full progress.
func TestStartCompletesSmallCopy(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "data.bin")
	writeFile(t, srcFile, 4096)
	dev := mountedDevice(t, false) // no sensor → cycle mode auto-selected

	u := newTestUI(t, &fakeLister{devices: []devices.Device{dev}})
	u.PollInterval = 0
	u.HandleDrop([]fyne.URI{fileURI(t, srcFile)})

	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, u, "done state", func() bool {
		return strings.HasSuffix(u.StateLabel.Text, u.I18N.T(ui.KeyStateDone))
	})
	if u.Running() {
		t.Fatal("run must be inactive after completion")
	}
	if got := u.ProgressBar.Value; got < 0.99 {
		t.Errorf("progress = %v, want ~1", got)
	}

	dstFile := filepath.Join(dev.MountPoint, "data.bin")
	if _, err := os.Stat(dstFile); err != nil {
		t.Errorf("copied file missing at %s: %v", dstFile, err)
	}
}
