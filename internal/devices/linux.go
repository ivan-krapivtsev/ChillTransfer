//go:build linux

package devices

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func listDevices(includeInternal bool) ([]Device, error) {
	out, err := exec.Command("lsblk", "-J", "-b").Output()
	if err != nil {
		return nil, fmt.Errorf("devices: lsblk: %w", err)
	}
	disks, err := parseLsblkJSON(out)
	if err != nil {
		return nil, err
	}

	mounts, err := readProcMounts()
	if err != nil {
		return nil, fmt.Errorf("devices: /proc/mounts: %w", err)
	}

	var devs []Device
	for _, b := range disks {
		if b.Type != "disk" {
			continue // partitions and loop devices are covered by their parent disk
		}
		if !includeInternal && !b.isRemovable() {
			continue
		}
		devs = append(devs, linuxDevice(b, mounts))
	}
	return devs, nil
}

// readProcMounts indexes real block-device mounts by device path.
func readProcMounts() (map[string]procMount, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	mounts := make(map[string]procMount)
	for _, m := range parseProcMounts(string(data)) {
		if strings.HasPrefix(m.Device, "/dev/") {
			mounts[m.Device] = m
		}
	}
	return mounts, nil
}

// linuxDevice builds a Device from one lsblk disk and the /proc/mounts index.
func linuxDevice(b lsblkBlock, mounts map[string]procMount) Device {
	d := Device{
		ID:        linuxDeviceID(b),
		Label:     b.Model,
		Removable: b.isRemovable(),
	}
	if b.SizeBytes != nil {
		d.CapacityBytes = *b.SizeBytes
	}

	// Primary mounted partition: the first child with a /proc/mounts entry.
	for _, c := range b.Children {
		m, ok := mounts["/dev/"+c.Name]
		if !ok {
			continue
		}
		d.MountPoint = m.MountPoint
		d.FileSystem = m.FSType
		if c.SizeBytes != nil && *c.SizeBytes > 0 {
			d.CapacityBytes = *c.SizeBytes
		}
		if d.Label == "" && c.Label != nil && *c.Label != "" {
			d.Label = *c.Label
		}
		break
	}

	// Last-resort label fallback: CI runners and some disks report no model or
	// partition label. A device without a label is unusable in the UI (FR-1),
	// so fall back to the kernel name.
	if d.Label == "" {
		d.Label = b.Name
	}

	if d.MountPoint != "" {
		var st syscall.Statfs_t
		if err := syscall.Statfs(d.MountPoint, &st); err == nil {
			blockSize := st.Bsize
			if st.Frsize > 0 {
				blockSize = st.Frsize
			}
			d.FreeBytes = st.Bavail * uint64(blockSize)
		}
	}

	// Temperature: best-effort hwmon correlation by model/serial (SPEC §4).
	// The smartctl -d usbsmart/sat fallback from SPEC §4 is deferred to v1.x.
	if path := findHwmonTempFile(b.Model, b.Serial); path != "" {
		d.TempSource = &hwmonTempSource{path: path}
	}
	return d
}

// linuxDeviceID builds a stable-enough ID for profile matching (FR-6): USB
// serial first, then model + size, then the kernel name.
func linuxDeviceID(b lsblkBlock) string {
	switch {
	case b.Serial != "":
		return "linux:" + b.Serial
	case b.Model != "":
		size := uint64(0)
		if b.SizeBytes != nil {
			size = *b.SizeBytes
		}
		return "linux:" + b.Model + ":" + strconv.FormatUint(size, 10)
	default:
		return "linux:" + b.Name
	}
}

// hwmonTempSource reads a /sys/class/hwmon/<dir>/temp1 file (millidegrees C).
type hwmonTempSource struct {
	path string
}

func (s *hwmonTempSource) Read() (float64, bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, false
	}
	return v / 1000.0, true
}

// findHwmonTempFile returns the temp1 path of an hwmon entry whose name
// contains the device model or serial (case-insensitive), or "" when nothing
// matches. Correlation by name is best-effort: most USB sticks expose no
// temperature sensor at all, in which case nil TempSource is correct.
func findHwmonTempFile(model, serial string) string {
	entries, err := os.ReadDir("/sys/class/hwmon")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		nameData, err := os.ReadFile(filepath.Join("/sys/class/hwmon", e.Name(), "name"))
		if err != nil {
			continue
		}
		hwmonName := strings.TrimSpace(string(nameData))
		if !hwmonMatches(hwmonName, model, serial) {
			continue
		}
		tempPath := filepath.Join("/sys/class/hwmon", e.Name(), "temp1")
		if _, err := os.Stat(tempPath); err == nil {
			return tempPath
		}
	}
	return ""
}

// hwmonMatches reports whether an hwmon name plausibly belongs to the device.
func hwmonMatches(hwmonName, model, serial string) bool {
	n := strings.ToLower(hwmonName)
	for _, cand := range []string{strings.ToLower(serial), strings.ToLower(model)} {
		if cand != "" && strings.Contains(n, cand) {
			return true
		}
	}
	return false
}
