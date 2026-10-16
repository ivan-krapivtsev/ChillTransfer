// Package ui implements the Fyne user interface (T-005).
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// DropZoneText is the placeholder label shown in the drop zone (T-005 replaces it).
const DropZoneText = "Перетащите файлы и папки сюда"

// NewWindow creates the main application window.
func NewWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("ChillTransfer")
	w.Resize(fyne.NewSize(760, 520))
	w.SetContent(BuildContent())
	return w
}

// BuildContent assembles the main window layout (placeholder until T-005).
func BuildContent() fyne.CanvasObject {
	dropZone := widget.NewLabelWithStyle(DropZoneText, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	title := widget.NewLabel("ChillTransfer")

	return container.NewVBox(
		container.NewPadded(container.NewHBox(title)),
		container.NewPadded(dropZone),
	)
}
