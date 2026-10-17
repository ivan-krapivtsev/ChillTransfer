package ui

import (
	"fmt"
	"path/filepath"
	"time"
)

// formatBytes renders a byte count in the largest convenient unit.
func formatBytes(n uint64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
		tb = 1 << 40
	)
	switch {
	case n >= tb:
		return fmt.Sprintf("%.1f ТБ", float64(n)/tb)
	case n >= gb:
		return fmt.Sprintf("%.1f ГБ", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f МБ", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f КБ", float64(n)/kb)
	default:
		return fmt.Sprintf("%d Б", n)
	}
}

// formatSpeed renders a throughput in bytes per second; zero means unknown.
func formatSpeed(bps float64) string {
	if bps <= 0 {
		return "—"
	}
	return formatBytes(uint64(bps)) + "/с"
}

// formatDuration renders an ETA-style duration; zero means unknown.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dч %dм", h, m)
	case m > 0:
		return fmt.Sprintf("%dм %dс", m, s)
	default:
		return fmt.Sprintf("%dс", s)
	}
}

// currentFileText shortens a source path to its base name for the progress line.
func currentFileText(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
