//go:build linux

package devices

import (
	"os/exec"
	"testing"
)

func TestHwmonMatches(t *testing.T) {
	cases := []struct {
		hwmon, model, serial string
		want                 bool
	}{
		{"usb_thermal_sensor", "SanDisk_Ultra_Flash_Drive", "4C531234567890123456", false},
		{"sanDisk_ultra_flash_drive_temp", "SanDisk_Ultra_Flash_Drive", "", true},
		{"temp_4c531234567890123456", "", "4C531234567890123456", true},
		{"coretemp", "Intel CPU", "", false},
		{"anything", "", "", false},
	}
	for _, c := range cases {
		if got := hwmonMatches(c.hwmon, c.model, c.serial); got != c.want {
			t.Errorf("hwmonMatches(%q,%q,%q) = %v, want %v", c.hwmon, c.model, c.serial, got, c.want)
		}
	}
}

func TestLinuxDeviceID(t *testing.T) {
	withSerial := linuxDeviceID(lsblkBlock{Name: "sdb", Serial: "ABC123"})
	if withSerial != "linux:ABC123" {
		t.Errorf("unexpected ID %q", withSerial)
	}
	size := uint64(31246464512)
	withModel := linuxDeviceID(lsblkBlock{Name: "sdb", Model: "SanDisk_Ultra_Flash_Drive", SizeBytes: &size})
	if withModel != "linux:SanDisk_Ultra_Flash_Drive:31246464512" {
		t.Errorf("unexpected ID %q", withModel)
	}
	fallback := linuxDeviceID(lsblkBlock{Name: "sdd"})
	if fallback != "linux:sdd" {
		t.Errorf("unexpected ID %q", fallback)
	}
}

// TestListDevicesLinuxSmoke is a best-effort integration check against real
// lsblk output; it must never fail for missing hardware.
func TestListDevicesLinuxSmoke(t *testing.T) {
	devs, err := ListDevices(true)
	if err != nil {
		t.Fatalf("ListDevices(true): %v", err)
	}
	for _, d := range devs {
		if d.ID == "" || d.Label == "" {
			t.Errorf("incomplete device: %+v", d)
		}
		if d.MountPoint != "" && d.CapacityBytes > 0 && d.FreeBytes > 2*d.CapacityBytes {
			t.Errorf("implausible free space: %+v", d)
		}
	}
	if len(devs) == 0 {
		t.Skip("no block devices reported by lsblk")
	}

	found, ok := FindDevice(devs[0].ID)
	if !ok || found.ID != devs[0].ID {
		t.Errorf("FindDevice(%q) = %v, %v; want the same device", devs[0].ID, found, ok)
	}
}

// TestListDevicesLinuxExternal verifies default-mode filtering when a
// removable device is connected.
func TestListDevicesLinuxExternal(t *testing.T) {
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

// TestParseLsblkJSONLive parses real lsblk output when the tool is available.
func TestParseLsblkJSONLive(t *testing.T) {
	out, err := exec.Command("lsblk", "-J", "-b").Output()
	if err != nil {
		t.Skipf("lsblk not available: %v", err)
	}
	devs, err := parseLsblkJSON(out)
	if err != nil {
		t.Fatalf("parseLsblkJSON(real output): %v", err)
	}
	if len(devs) == 0 {
		t.Skip("no block devices")
	}
	for _, d := range devs {
		if d.Name == "" || d.Type == "" {
			t.Errorf("incomplete real device: %+v", d)
		}
	}
}
