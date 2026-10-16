package devices

import (
	"encoding/json"
	"fmt"
	"strings"
)

// lsblkBlock is one node of the `lsblk -J -b` device tree. SizeBytes is a
// pointer because lsblk omits "size" for some devices; with the -b flag it
// is a byte count (without -b it would be human-readable, which we never pass).
type lsblkBlock struct {
	Name       string          `json:"name"`
	MajMin     string          `json:"maj:min"`
	SizeBytes  *uint64         `json:"size"`
	Type       string          `json:"type"`
	Removable  json.RawMessage `json:"removable"` // JSON bool or "true"/"1" string depending on lsblk/udev version
	Model      string          `json:"model"`
	Serial     string          `json:"serial"`
	Label      *string         `json:"label"`
	Mountpoint *string         `json:"mountpoint"`
	Children   []lsblkBlock    `json:"children"`
}

// isRemovable tolerates both the JSON boolean and the legacy string forms of
// the removable flag.
func (b lsblkBlock) isRemovable() bool {
	if len(b.Removable) == 0 {
		return false
	}
	var v bool
	if err := json.Unmarshal(b.Removable, &v); err == nil {
		return v
	}
	var s string
	if err := json.Unmarshal(b.Removable, &s); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return true
	default:
		return false
	}
}

// parseLsblkJSON parses the output of `lsblk -J -b` and returns the top-level
// block devices (disks, with partitions nested in Children).
func parseLsblkJSON(data []byte) ([]lsblkBlock, error) {
	var root struct {
		Blockdevices []lsblkBlock `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("devices: lsblk: %w", err)
	}
	return root.Blockdevices, nil
}

// procMount is one line of /proc/mounts.
type procMount struct {
	Device     string
	MountPoint string
	FSType     string
}

// parseProcMounts parses /proc/mounts content; every line with at least two
// fields yields an entry (pseudo-filesystems are filtered by callers).
func parseProcMounts(content string) []procMount {
	var mounts []procMount
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mounts = append(mounts, procMount{
			Device:     unescapeMountPath(fields[0]),
			MountPoint: unescapeMountPath(fields[1]),
			FSType:     fields[2],
		})
	}
	return mounts
}

// unescapeMountPath decodes the octal escapes /proc/mounts uses for spaces,
// tabs and newlines in device paths and mount points (e.g. "My\040Stick").
func unescapeMountPath(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			b.WriteByte('\\') // lone trailing backslash: keep as-is
			break
		}
		if s[i] >= '0' && s[i] <= '7' {
			oct := int(s[i] - '0')
			j := i + 1
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				oct = oct*8 + int(s[j]-'0')
				j++
			}
			b.WriteByte(byte(oct))
			i = j - 1
		} else {
			b.WriteByte(s[i]) // "\x" is a literal x in /proc/mounts
		}
	}
	return b.String()
}
