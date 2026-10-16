//go:build windows

package devices

import "testing"

// TestListDevicesWindowsSmoke is a best-effort integration check against real
// drive enumeration; it must never fail for missing hardware.
func TestListDevicesWindowsSmoke(t *testing.T) {
	devs, err := ListDevices(true)
	if err != nil {
		t.Fatalf("ListDevices(true): %v", err)
	}
	for _, d := range devs {
		if d.ID == "" || d.MountPoint == "" {
			t.Errorf("incomplete device: %+v", d)
		}
		if d.FreeBytes > 2*d.CapacityBytes && d.CapacityBytes > 0 {
			t.Errorf("implausible free space: %+v", d)
		}
	}
	if len(devs) == 0 {
		t.Skip("no local drives found")
	}

	found, ok := FindDevice(devs[0].ID)
	if !ok || found.ID != devs[0].ID {
		t.Errorf("FindDevice(%q) = %v, %v; want the same device", devs[0].ID, found, ok)
	}
}

// TestListDevicesWindowsExternal verifies default-mode filtering when a
// removable device is connected.
func TestListDevicesWindowsExternal(t *testing.T) {
	devs, err := ListDevices(false)
	if err != nil {
		t.Fatalf("ListDevices(false): %v", err)
	}
	if len(devs) == 0 {
		t.Skip("no removable device connected")
	}
	for _, d := range devs {
		if !d.Removable {
			t.Errorf("default mode returned a non-removable device: %+v", d)
		}
	}
}

func TestCstrAt(t *testing.T) {
	buf := []byte{0, 0, 'W', 'D', '-', 'A', '1', 0, 'x'}
	if got := cstrAt(buf, 2); got != "WD-A1" {
		t.Errorf("cstrAt = %q, want WD-A1", got)
	}
	if got := cstrAt(buf, 8); got != "x" {
		t.Errorf("cstrAt unterminated = %q, want x", got)
	}
	for _, off := range []int{0, -1, len(buf), len(buf) + 5} {
		if got := cstrAt(buf, off); got != "" {
			t.Errorf("cstrAt(off=%d) = %q, want empty", off, got)
		}
	}
}
