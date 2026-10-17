package devices

import "github.com/ivan-krapivtsev/chilltransfer/internal/engine"

// Device describes a storage device that can be a copy target (SPEC FR-1).
type Device struct {
	// ID is a stable identifier used to match saved profiles (FR-6):
	// USB serial on Windows, media name + size on macOS, model/serial or
	// kernel name on Linux.
	ID string

	// Label is the human-readable device or volume name for UI display.
	Label string

	// CapacityBytes and FreeBytes are the total and free space of the
	// primary mounted volume (whole-disk size when no volume is mounted).
	CapacityBytes uint64
	FreeBytes     uint64

	// FileSystem is the file system type of the primary volume ("", "exFAT", ...).
	FileSystem string

	// Removable reports whether the device can be unplugged.
	Removable bool

	// MountPoint is where the primary volume is mounted; empty means not
	// mounted. The UI filters out devices with an empty MountPoint from
	// selectable targets (approved decision, T-004).
	MountPoint string

	// TempSource provides temperature readings when a sensor was found;
	// nil means no sensor and the engine falls back to cyclic mode (SPEC §4).
	TempSource engine.TemperatureSource
}

// ListDevices returns storage devices. When includeInternal is false only
// removable/external devices are returned (FR-1: internal disks are hidden
// by default, shown via an explicit "show all" option).
func ListDevices(includeInternal bool) ([]Device, error) {
	return listDevices(includeInternal)
}

// FindDevice returns the device with the given ID, if present. It scans all
// devices including internal ones so that saved profiles keep resolving.
func FindDevice(id string) (*Device, bool) {
	devs, err := ListDevices(true)
	if err != nil {
		return nil, false
	}
	for i := range devs {
		if devs[i].ID == id {
			return &devs[i], true
		}
	}
	return nil, false
}
