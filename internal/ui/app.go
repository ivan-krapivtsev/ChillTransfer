// Package ui implements the Fyne user interface for ChillTransfer (T-005):
// device selection, drop zone and queue, mode settings and run controls
// (SPEC §6). Dependencies (device lister, clock) are injected so headless
// tests never touch real hardware.
package ui

import "fyne.io/fyne/v2"

// DropZoneText is the label shown in the drop zone.
const DropZoneText = "Перетащите файлы и папки сюда"

// NewWindow creates the main application window wired to production
// dependencies (platform device enumeration, real clock). Tests use New with
// an injected DeviceLister instead.
func NewWindow(a fyne.App) fyne.Window {
	return New(a, ProductionLister{}).Window
}
