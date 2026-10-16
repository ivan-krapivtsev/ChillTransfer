//go:build darwin

package devices

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func listDevices(includeInternal bool) ([]Device, error) {
	out, err := runDiskutil("list", "-plist")
	if err != nil {
		return nil, fmt.Errorf("devices: diskutil list: %w", err)
	}
	ids, err := parseDiskutilListPlist(out)
	if err != nil {
		return nil, err
	}

	var devs []Device
	for _, id := range ids {
		if !isWholeDiskIdentifier(id) {
			continue // partitions and APFS volumes are covered by their whole disk
		}
		infoOut, err := runDiskutil("info", "-plist", id)
		if err != nil {
			continue // device vanished (unplug race) or unreadable — best-effort skip
		}
		info, err := parseDiskutilInfoPlist(infoOut)
		if err != nil {
			continue
		}
		if !includeInternal && !info.isExternal() {
			continue
		}

		vols := info.Volumes
		if len(vols) == 0 {
			// `diskutil info -plist <whole disk>` does not always report a
			// Volumes array (observed on macOS 26 for internal disks); fall
			// back to querying the disk's partitions individually.
			vols = mountedVolumesOf(id, ids)
		}
		devs = append(devs, deviceFromDiskutilInfo(info, vols))
	}
	return devs, nil
}

func runDiskutil(args ...string) ([]byte, error) {
	out, err := exec.Command("diskutil", args...).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// mountedVolumesOf queries each partition of the whole disk (identifiers are
// "<wholeDisk>s<N>...") and returns those that have a mount point.
func mountedVolumesOf(wholeDisk string, allIDs []string) []diskutilVolume {
	prefix := wholeDisk + "s"
	var vols []diskutilVolume
	for _, id := range allIDs {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		out, err := runDiskutil("info", "-plist", id)
		if err != nil {
			continue
		}
		info, err := parseDiskutilInfoPlist(out)
		if err != nil || info.MountPoint == "" {
			continue
		}
		vols = append(vols, diskutilVolume{
			VolumeName: info.VolumeName,
			MountPoint: info.MountPoint,
			VolumeUUID: info.VolumeUUID,
			TotalSize:  info.TotalSize,
			FileSystem: info.FileSystem,
		})
	}
	return vols
}

// deviceFromDiskutilInfo builds a Device from whole-disk info and its mounted volumes.
func deviceFromDiskutilInfo(info diskutilInfo, vols []diskutilVolume) Device {
	d := Device{
		ID:            macosDeviceID(info),
		Label:         info.MediaName,
		CapacityBytes: info.Size,
		Removable:     info.Removable || info.Ejectable,
	}

	for _, v := range vols {
		// Any volume name improves display; mount data comes from the primary.
		if d.Label == "" && v.VolumeName != "" {
			d.Label = v.VolumeName
		}
		if v.MountPoint == "" {
			continue
		}
		d.MountPoint = v.MountPoint
		if v.TotalSize > 0 {
			d.CapacityBytes = v.TotalSize
		}
		if v.FileSystem != "" {
			d.FileSystem = v.FileSystem
		}
		break // primary volume: the first mounted one
	}
	if d.Label == "" {
		d.Label = info.DeviceIdentifier
	}

	if d.MountPoint != "" {
		var st syscall.Statfs_t
		if err := syscall.Statfs(d.MountPoint, &st); err == nil {
			d.FreeBytes = st.Bavail * uint64(st.Bsize)
		}
	}

	// TODO(T-004 follow-up): temperature via IOKit — query the IOService of
	// the device's USB parent for a temperature property (cgo/ObjC, darwin-only).
	// Not attempted in T-004: cgo breaks pure-Go cross-compilation. nil means
	// "no sensor" and the engine falls back to cyclic mode (SPEC §4).
	d.TempSource = nil
	return d
}

// macosDeviceID builds a stable-enough ID for profile matching (FR-6): media
// name + size, falling back to the kernel identifier when no media name is
// reported (typical for internal disks and APFS containers).
func macosDeviceID(info diskutilInfo) string {
	if info.MediaName != "" {
		return "macos:" + info.MediaName + ":" + strconv.FormatUint(info.Size, 10)
	}
	return "macos:" + info.DeviceIdentifier
}
