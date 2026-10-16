package ui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

// TestBuildContentSmoke verifies the placeholder layout contains the drop-zone label.
func TestBuildContentSmoke(t *testing.T) {
	root := ui.BuildContent()
	if !containsLabel(root, ui.DropZoneText) {
		t.Fatalf("drop zone label %q not found in content", ui.DropZoneText)
	}
}

// containsLabel walks the widget tree looking for a label with exact text.
func containsLabel(obj fyne.CanvasObject, text string) bool {
	if l, ok := obj.(*widget.Label); ok && l.Text == text {
		return true
	}
	for _, child := range childrenOf(obj) {
		if containsLabel(child, text) {
			return true
		}
	}
	return false
}

func childrenOf(obj fyne.CanvasObject) []fyne.CanvasObject {
	if c, ok := obj.(*fyne.Container); ok {
		return c.Objects
	}
	return nil
}
