package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ivan-krapivtsev/chilltransfer/internal/devices"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 Б"},
		{512, "512 Б"},
		{1 << 10, "1.0 КБ"},
		{1536, "1.5 КБ"},
		{1 << 20, "1.0 МБ"},
		{3 << 30, "3.0 ГБ"},
		{2 << 40, "2.0 ТБ"},
	}
	for _, tc := range cases {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeviceOptionText(t *testing.T) {
	d := devices.Device{ID: "usb-1", Label: "SanDisk 32GB", CapacityBytes: 32 << 30, FreeBytes: 20 << 30, FileSystem: "exFAT", MountPoint: "/Volumes/SanDisk"}
	if got := deviceOptionText(d); got != "SanDisk 32GB — 32.0 ГБ / свободно 20.0 ГБ, exFAT" {
		t.Errorf("deviceOptionText = %q", got)
	}

	d.MountPoint = "" // unmounted devices are marked but stay visible
	if !strings.HasSuffix(deviceOptionText(d), "(не смонтировано)") {
		t.Errorf("unmarked unmounted device: %q", deviceOptionText(d))
	}

	// Empty label falls back to ID; empty file system renders as "—".
	d2 := devices.Device{ID: "usb-9", CapacityBytes: 1 << 30, MountPoint: "/Volumes/USB9"}
	if got := deviceOptionText(d2); got != "usb-9 — 1.0 ГБ / свободно 0 Б, —" {
		t.Errorf("deviceOptionText fallback = %q", got)
	}
}

func TestFormatSpeedAndDuration(t *testing.T) {
	if got := formatSpeed(0); got != "—" {
		t.Errorf("formatSpeed(0) = %q, want —", got)
	}
	if got := formatSpeed(85 << 20); got != "85.0 МБ/с" {
		t.Errorf("formatSpeed(85MB/s) = %q", got)
	}

	if got := formatDuration(0); got != "—" {
		t.Errorf("formatDuration(0) = %q, want —", got)
	}
	if got := formatDuration(4*time.Minute + 32*time.Second); got != "4м 32с" {
		t.Errorf("formatDuration(4m32s) = %q", got)
	}
	if got := formatDuration(time.Hour + 5*time.Minute); got != "1ч 5м" {
		t.Errorf("formatDuration(1h5m) = %q", got)
	}
}
