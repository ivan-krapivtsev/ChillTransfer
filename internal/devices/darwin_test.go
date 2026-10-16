//go:build darwin

package devices

import "testing"

// TestListDevicesDarwinSmoke is a best-effort integration check against real
// diskutil output; it must never fail for missing hardware.
func TestListDevicesDarwinSmoke(t *testing.T) {
	devs, err := ListDevices(true)
	if err != nil {
		t.Fatalf("ListDevices(true): %v", err)
	}
	for _, d := range devs {
		if d.ID == "" {
			t.Errorf("device with empty ID: %+v", d)
		}
		if d.MountPoint != "" && d.CapacityBytes > 0 && d.FreeBytes > 2*d.CapacityBytes {
			t.Errorf("implausible free space: %+v", d)
		}
	}
	if len(devs) == 0 {
		t.Skip("no disks reported by diskutil")
	}

	found, ok := FindDevice(devs[0].ID)
	if !ok || found.ID != devs[0].ID {
		t.Errorf("FindDevice(%q) = %v, %v; want the same device", devs[0].ID, found, ok)
	}
}

// TestListDevicesDarwinExternal verifies default-mode filtering when an
// external device is connected.
func TestListDevicesDarwinExternal(t *testing.T) {
	devs, err := ListDevices(false)
	if err != nil {
		t.Fatalf("ListDevices(false): %v", err)
	}
	if len(devs) == 0 {
		t.Skip("no external/removable device connected")
	}
	for _, d := range devs {
		if !d.Removable {
			t.Errorf("default mode returned a non-removable device: %+v", d)
		}
	}
}
