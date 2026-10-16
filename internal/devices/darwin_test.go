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

func TestMacosDeviceID(t *testing.T) {
	withName := macosDeviceID(diskutilInfo{MediaName: "SanDisk Ultra", Size: 128, DeviceIdentifier: "disk4"})
	if withName != "macos:SanDisk Ultra:128" {
		t.Errorf("unexpected ID %q", withName)
	}
	noName := macosDeviceID(diskutilInfo{DeviceIdentifier: "disk0"})
	if noName != "macos:disk0" {
		t.Errorf("unexpected fallback ID %q", noName)
	}
}

func TestDeviceFromDiskutilInfo(t *testing.T) {
	info, err := parseDiskutilInfoPlist([]byte(usbStickInfoPlistFixture))
	if err != nil {
		t.Fatalf("parseDiskutilInfoPlist: %v", err)
	}
	d := deviceFromDiskutilInfo(info, info.Volumes)
	if d.ID == "" || d.Label != "SanDisk Ultra" || !d.Removable {
		t.Errorf("unexpected device: %+v", d)
	}
	if d.MountPoint != "/Volumes/STICK" {
		t.Errorf("MountPoint = %q, want /Volumes/STICK (Statfs may fail in test env)", d.MountPoint)
	}
	if d.CapacityBytes != 127984536064 || d.FileSystem != "exFAT" {
		t.Errorf("volume capacity/fs not applied: %+v", d)
	}
	if d.TempSource != nil {
		t.Error("TempSource must be nil until the IOKit follow-up lands")
	}

	unmounted := deviceFromDiskutilInfo(info, nil)
	if unmounted.MountPoint != "" || unmounted.CapacityBytes != 128000000000 {
		t.Errorf("unmounted device should keep whole-disk size and empty mount: %+v", unmounted)
	}
}

func TestDeviceFromDiskutilInfoLabelFallback(t *testing.T) {
	d := deviceFromDiskutilInfo(diskutilInfo{DeviceIdentifier: "disk7"}, []diskutilVolume{{VolumeName: "NO NAME"}})
	if d.Label != "NO NAME" {
		t.Errorf("label should fall back to volume name, got %q", d.Label)
	}
	d = deviceFromDiskutilInfo(diskutilInfo{DeviceIdentifier: "disk7"}, nil)
	if d.Label != "disk7" {
		t.Errorf("label should fall back to identifier, got %q", d.Label)
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
