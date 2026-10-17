package ui

import (
	"fmt"

	"github.com/ivan-krapivtsev/chilltransfer/internal/devices"
)

// DeviceLister abstracts device enumeration so UI tests can inject a fake
// provider instead of touching real hardware (T-005 testability requirement).
type DeviceLister interface {
	List(includeInternal bool) ([]devices.Device, error)
}

// ProductionLister is the default DeviceLister backed by the platform device layer.
type ProductionLister struct{}

func (ProductionLister) List(includeInternal bool) ([]devices.Device, error) {
	return devices.ListDevices(includeInternal)
}

// deviceOptionText renders one device as a dropdown option: name, capacity,
// free space and file system. Devices without a mounted volume are marked
// explicitly — they stay visible but are not selectable copy targets (T-004).
func deviceOptionText(d devices.Device) string {
	label := d.Label
	if label == "" {
		label = d.ID
	}
	fs := d.FileSystem
	if fs == "" {
		fs = "—"
	}
	text := fmt.Sprintf("%s — %s / свободно %s, %s", label, formatBytes(d.CapacityBytes), formatBytes(d.FreeBytes), fs)
	if d.MountPoint == "" {
		text += " (не смонтировано)"
	}
	return text
}
