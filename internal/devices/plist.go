package devices

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// plistValue is a node of a parsed property list. Fields are zero-valued for
// types the node does not carry, so missing keys degrade to zero values.
type plistValue struct {
	str   string  // <string>, <date> (raw text), <data> (base64 text)
	intv  int64   // <integer>
	float float64 // <real>
	boolv bool    // <true/> / <false/>
	array []plistValue
	dict  map[string]plistValue
}

// plistParser is a token-based reader for the XML property list format used
// by `diskutil -plist`. It is intentionally small and dependency-free.
type plistParser struct {
	d *xml.Decoder
}

func newPlistParser(data []byte) *plistParser {
	d := xml.NewDecoder(bytes.NewReader(data))
	// diskutil output carries a DOCTYPE declaration; Strict mode rejects it.
	d.Strict = false
	return &plistParser{d: d}
}

// parsePlist parses one complete plist document and returns its root value.
func parsePlist(data []byte) (plistValue, error) {
	p := newPlistParser(data)
	// Skip prolog/doctype/comments until the <plist> start element.
	for {
		tok, err := p.d.Token()
		if err != nil {
			return plistValue{}, fmt.Errorf("devices: parse plist: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "plist" {
			break
		}
	}
	v, found, err := p.parseChild()
	if err != nil {
		return plistValue{}, fmt.Errorf("devices: parse plist: %w", err)
	}
	if !found {
		return plistValue{}, fmt.Errorf("devices: parse plist: missing root value")
	}
	return v, nil
}

// parseChild parses the next value element inside the current container. It
// reports found=false when it consumes the container's end tag instead.
func (p *plistParser) parseChild() (v plistValue, found bool, err error) {
	for {
		tok, err := p.d.Token()
		if err != nil {
			return plistValue{}, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			v, err = p.parseElement(t.Name.Local)
			return v, true, err
		case xml.EndElement:
			return plistValue{}, false, nil
		default:
			continue // whitespace and comments between elements
		}
	}
}

func (p *plistParser) parseElement(name string) (plistValue, error) {
	switch name {
	case "string", "date":
		s, err := p.readText()
		return plistValue{str: s}, err
	case "integer":
		s, err := p.readText()
		if err != nil {
			return plistValue{}, err
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return plistValue{}, fmt.Errorf("devices: parse plist: bad integer %q", s)
		}
		return plistValue{intv: n}, nil
	case "real":
		s, err := p.readText()
		if err != nil {
			return plistValue{}, err
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return plistValue{}, fmt.Errorf("devices: parse plist: bad real %q", s)
		}
		return plistValue{float: f}, nil
	case "true":
		if _, err := p.readText(); err != nil {
			return plistValue{}, err
		}
		return plistValue{boolv: true}, nil
	case "false":
		if _, err := p.readText(); err != nil {
			return plistValue{}, err
		}
		return plistValue{}, nil
	case "data":
		s, err := p.readText()
		return plistValue{str: s}, err
	case "array":
		var arr []plistValue
		for {
			v, ok, err := p.parseChild()
			if err != nil {
				return plistValue{}, err
			}
			if !ok {
				break
			}
			arr = append(arr, v)
		}
		return plistValue{array: arr}, nil
	case "dict":
		dict := map[string]plistValue{}
		var key string
		haveKey := false
		for {
			tok, err := p.d.Token()
			if err != nil {
				return plistValue{}, err
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				if _, isEnd := tok.(xml.EndElement); isEnd {
					break
				}
				continue
			}
			var v plistValue
			switch se.Name.Local {
			case "key":
				// The following element is this key's value.
				key, err = p.readText()
				haveKey = true
			default:
				v, err = p.parseElement(se.Name.Local)
				if haveKey {
					dict[key] = v
					haveKey = false
				}
			}
			if err != nil {
				return plistValue{}, err
			}
		}
		return plistValue{dict: dict}, nil
	default:
		// Unknown element: skip it so a foreign key never breaks parsing.
		if err := p.skipToEnd(name); err != nil {
			return plistValue{}, err
		}
		return plistValue{}, nil
	}
}

// readText consumes character data until the end tag of the current element
// and returns its trimmed content (empty for self-closing elements).
func (p *plistParser) readText() (string, error) {
	var sb strings.Builder
	for {
		tok, err := p.d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			return strings.TrimSpace(sb.String()), nil
		}
	}
}

// skipToEnd consumes tokens until the end tag of the named element.
func (p *plistParser) skipToEnd(name string) error {
	for {
		tok, err := p.d.Token()
		if err != nil {
			return err
		}
		if t, ok := tok.(xml.EndElement); ok && t.Name.Local == name {
			return nil
		}
	}
}

// wholeDiskID matches identifiers of whole disks (disk0, disk4, ...), as
// opposed to partitions and APFS volumes (disk4s1, disk3s5, ...).
var wholeDiskID = regexp.MustCompile(`^disk[0-9]+$`)

func isWholeDiskIdentifier(id string) bool { return wholeDiskID.MatchString(id) }

// diskutilVolume mirrors the per-volume keys of `diskutil info -plist`.
type diskutilVolume struct {
	VolumeName string
	MountPoint string
	VolumeUUID string
	TotalSize  uint64
	FileSystem string // FilesystemName key; empty when not reported
}

// diskutilInfo mirrors the device-level keys of `diskutil info -plist
// <identifier>`. The last five fields are populated only when a volume or
// partition identifier (rather than a whole disk) was queried.
type diskutilInfo struct {
	DeviceIdentifier               string
	MediaType                      string
	BusProtocol                    string
	Internal                       bool
	Removable                      bool
	Ejectable                      bool
	RemovableMediaOrExternalDevice bool
	MediaName                      string
	Size                           uint64
	WholeDisk                      bool
	Volumes                        []diskutilVolume

	VolumeName string
	MountPoint string
	VolumeUUID string
	TotalSize  uint64
	FileSystem string // FilesystemName key; empty when not reported
}

// parseDiskutilListPlist parses `diskutil list -plist` and returns all device
// identifiers (whole disks, partitions and APFS volumes).
func parseDiskutilListPlist(data []byte) ([]string, error) {
	root, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("devices: diskutil list: %w", err)
	}
	var ids []string
	for _, v := range root.dict["AllDisks"].array {
		ids = append(ids, v.str)
	}
	return ids, nil
}

// parseDiskutilInfoPlist parses `diskutil info -plist <identifier>`.
func parseDiskutilInfoPlist(data []byte) (diskutilInfo, error) {
	root, err := parsePlist(data)
	if err != nil {
		return diskutilInfo{}, fmt.Errorf("devices: diskutil info: %w", err)
	}
	dict := root.dict
	info := diskutilInfo{
		DeviceIdentifier:               dict["DeviceIdentifier"].str,
		MediaType:                      dict["MediaType"].str,
		BusProtocol:                    dict["BusProtocol"].str,
		Internal:                       dict["Internal"].boolv,
		Removable:                      dict["Removable"].boolv,
		Ejectable:                      dict["Ejectable"].boolv,
		RemovableMediaOrExternalDevice: dict["RemovableMediaOrExternalDevice"].boolv,
		MediaName:                      dict["MediaName"].str,
		Size:                           uint64(dict["Size"].intv),
		WholeDisk:                      dict["WholeDisk"].boolv,
		VolumeName:                     dict["VolumeName"].str,
		MountPoint:                     dict["MountPoint"].str,
		VolumeUUID:                     dict["VolumeUUID"].str,
		TotalSize:                      uint64(dict["TotalSize"].intv),
		FileSystem:                     dict["FilesystemName"].str,
	}
	if vols := dict["Volumes"]; len(vols.array) > 0 {
		for _, v := range vols.array {
			vd := v.dict
			info.Volumes = append(info.Volumes, diskutilVolume{
				VolumeName: vd["VolumeName"].str,
				MountPoint: vd["MountPoint"].str,
				VolumeUUID: vd["VolumeUUID"].str,
				TotalSize:  uint64(vd["TotalSize"].intv),
				FileSystem: vd["FilesystemName"].str,
			})
		}
	}
	return info, nil
}

// isExternal reports whether the device is an external/removable one per
// SPEC FR-1. Internal disks are always excluded; everything else needs at
// least one explicit external flag or an external bus protocol.
func (i diskutilInfo) isExternal() bool {
	if i.Internal {
		return false
	}
	return i.Removable || i.Ejectable || i.RemovableMediaOrExternalDevice ||
		isExternalBusProtocol(i.BusProtocol)
}

// isExternalBusProtocol matches BusProtocol values that indicate an external bus.
func isExternalBusProtocol(bus string) bool {
	b := strings.ToLower(strings.TrimSpace(bus))
	return strings.HasPrefix(b, "usb") || strings.HasPrefix(b, "thunderbolt")
}
