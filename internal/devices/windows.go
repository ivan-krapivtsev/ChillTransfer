//go:build windows

package devices

import (
	"bytes"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// Storage IOCTL codes (CTL_CODE with device type IOCTL_STORAGE_BASE = 0x2D).
	ioctlStorageGetDeviceNumber = 0x002D1080 // CTL_CODE(0x2D, 0x420, METHOD_BUFFERED, FILE_ANY_ACCESS)
	ioctlStorageQueryProperty   = 0x002D1400 // CTL_CODE(0x2D, 0x500, METHOD_BUFFERED, FILE_ANY_ACCESS)

	// STORAGE_BUS_TYPE values used for external-bus detection (winnt.h).
	busTypeUnknown = 0
	busType1394    = 5
	busTypeUSB     = 19
)

// storageDeviceNumber is the response of IOCTL_STORAGE_GET_DEVICE_NUMBER.
type storageDeviceNumber struct {
	DeviceType      uint16
	DeviceNumber    uint16
	PartitionNumber uint16
}

// storageDeviceDescriptor is the fixed header of STORAGE_DEVICE_DESCRIPTOR;
// the vendor/product/serial strings live at their offset members in the raw buffer.
type storageDeviceDescriptor struct {
	Version               uint32
	Size                  uint32
	DeviceType            byte
	DeviceTypeModifier    byte
	RemovableMedia        byte
	CommandQueueing       byte
	VendorIdOffset        uint32
	ProductIdOffset       uint32
	ProductRevisionOffset uint32
	SerialNumberOffset    uint32
	BusType               uint32
	BusTypeOffset         uint32
}

// storageQuery carries what queryStorageDevice learned from the IOCTLs.
type storageQuery struct {
	busType      uint32
	serial       string
	productID    string
	deviceNumber uint16
}

func listDevices(includeInternal bool) ([]Device, error) {
	drives, err := logicalDrives()
	if err != nil {
		return nil, fmt.Errorf("devices: GetLogicalDriveStrings: %w", err)
	}

	var devs []Device
	for _, root := range drives {
		dt := windows.GetDriveType(windows.StringToUTF16Ptr(root))
		switch dt {
		case windows.DRIVE_REMOVABLE, windows.DRIVE_FIXED:
		default:
			continue // unknown/empty/network/optical — not a copy target (FR-1)
		}
		d := describeDrive(root, dt)
		if !includeInternal && !d.Removable {
			continue
		}
		devs = append(devs, d)
	}
	return devs, nil
}

// logicalDrives returns the root paths of all logical drives ("C:\\", ...).
func logicalDrives() ([]string, error) {
	buf := make([]uint16, 512)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil {
		return nil, err
	}
	var drives []string
	for _, s := range strings.Split(windows.UTF16ToString(buf[:n]), "\x00") {
		if s != "" {
			drives = append(drives, s)
		}
	}
	return drives, nil
}

// describeDrive fills in everything that is known about one logical drive.
func describeDrive(root string, driveType uint32) Device {
	d := Device{MountPoint: root} // Windows volumes are always mounted at their letter

	if volName, fsName, err := volumeInfo(root); err == nil {
		d.Label = volName
		d.FileSystem = fsName
	}

	var total, free uint64
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(root), &free, &total, nil); err == nil {
		d.CapacityBytes = total
		d.FreeBytes = free
	}

	q, ok := queryStorageDevice(root)
	if ok {
		if q.serial != "" {
			d.ID = "win:" + q.serial // stable across reboots (approved decision, T-004)
		} else {
			d.ID = fmt.Sprintf("win:physicaldrive%d", q.deviceNumber)
		}
		if d.Label == "" && q.productID != "" {
			d.Label = q.productID
		}
		// External USB SSD/HDD report DRIVE_FIXED, so the bus type decides.
		d.Removable = driveType == windows.DRIVE_REMOVABLE ||
			q.busType == busTypeUSB || q.busType == busType1394
	} else if driveType == windows.DRIVE_REMOVABLE {
		// IOCTLs failed (rare); the drive type alone still says removable.
		d.ID = "win:" + root[:1] // letter-based fallback, not stable across reboots
		d.Removable = true
	}

	// Last-resort ID fallback: IOCTLs failed and the drive is not removable.
	// The letter-based ID is not stable across reboots, but a device without
	// an ID can never match saved profiles (FR-6), so completeness wins.
	if d.ID == "" {
		d.ID = "win:" + root[:1]
	}
	if d.Label == "" {
		d.Label = root
	}

	// TODO(T-004 follow-up): correlate a USB HID temperature sensor (USB
	// Temperature Sensor Class, usage page 0x02) with this device via hid.dll
	// and the parent USB path; nil until implemented. The engine then falls
	// back to cyclic mode (SPEC §4).
	d.TempSource = nil
	return d
}

// volumeInfo returns the volume label and file system name of a drive root.
func volumeInfo(root string) (label, fsName string, err error) {
	var volBuf [260]uint16
	var serial uint32
	var maxComponentLength, fsFlags uint32
	var fsBuf [260]uint16
	err = windows.GetVolumeInformation(
		windows.StringToUTF16Ptr(root),
		&volBuf[0], uint32(len(volBuf)), &serial, &maxComponentLength, &fsFlags,
		&fsBuf[0], uint32(len(fsBuf)))
	if err != nil {
		return "", "", err
	}
	return windows.UTF16ToString(volBuf[:]), windows.UTF16ToString(fsBuf[:]), nil
}

// queryStorageDevice opens the physical drive behind a letter and reads its
// bus type, serial and product ID. ok is false when any step fails; callers
// must treat that as "unknown" rather than an error (best-effort per SPEC §4).
func queryStorageDevice(root string) (storageQuery, bool) {
	var q storageQuery
	path := fmt.Sprintf(`\\.\%c:`, root[0])

	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path), 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return q, false
	}
	defer windows.Close(h)

	var num storageDeviceNumber
	var ret uint32
	err = windows.DeviceIoControl(h, ioctlStorageGetDeviceNumber, nil, 0,
		(*byte)(unsafe.Pointer(&num)), uint32(unsafe.Sizeof(num)), &ret, nil)
	if err != nil {
		return q, false
	}
	q.deviceNumber = num.DeviceNumber

	var query [4]byte // STORAGE_PROPERTY_QUERY{StorageDeviceProperty=0, PropertyStandardQuery=0}
	descBuf := make([]byte, 1024)
	err = windows.DeviceIoControl(h, ioctlStorageQueryProperty, &query[0], uint32(len(query)),
		&descBuf[0], uint32(len(descBuf)), &ret, nil)
	if err != nil || int(ret) < int(unsafe.Sizeof(storageDeviceDescriptor{})) {
		return q, false
	}

	desc := *(*storageDeviceDescriptor)(unsafe.Pointer(&descBuf[0]))
	q.busType = desc.BusType
	q.serial = cstrAt(descBuf[:ret], int(desc.SerialNumberOffset))
	q.productID = cstrAt(descBuf[:ret], int(desc.ProductIdOffset))
	return q, true
}

// cstrAt reads a NUL-terminated ASCII string at offset off in b.
func cstrAt(b []byte, off int) string {
	if off <= 0 || off >= len(b) {
		return ""
	}
	s := b[off:]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}
