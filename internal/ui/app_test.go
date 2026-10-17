package ui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

// TestNewWindowSmoke verifies the production entry point builds a window whose
// content contains the drop zone. Device enumeration is best-effort (real
// hardware), so device options are not asserted here — see ui_test.go for the
// injected-provider coverage.
func TestNewWindowSmoke(t *testing.T) {
	w := ui.NewWindow(test.NewApp())
	if w == nil || w.Content() == nil {
		t.Fatal("window or content is nil")
	}
	if !containsLabel(w.Content(), ui.DropZoneText) {
		t.Fatalf("drop zone label %q not found in window content", ui.DropZoneText)
	}
}

// TestContentLayoutContainsRequiredSections verifies the SPEC §6 layout: device
// panel on top, drop zone + queue in the middle, mode settings and controls at
// the bottom.
func TestContentLayoutContainsRequiredSections(t *testing.T) {
	u := newTestUI(t, &fakeLister{})
	root := u.Content()

	for _, text := range []string{ui.DropZoneText, "Устройство:", "Очередь:", "Режим:"} {
		if !containsLabel(root, text) {
			t.Errorf("label %q not found in content", text)
		}
	}
	if u.StartButton.Text != "Старт" || u.PauseButton.Text != "Пауза" || u.StopButton.Text != "Стоп" {
		t.Errorf("control labels = %q/%q/%q, want Старт/Пауза/Стоп", u.StartButton.Text, u.PauseButton.Text, u.StopButton.Text)
	}
	for _, opt := range []string{ui.ModeSensor, ui.ModeCycle} {
		found := false
		for _, o := range u.ModeRadio.Options {
			if o == opt {
				found = true
			}
		}
		if !found {
			t.Errorf("mode option %q missing from radio group", opt)
		}
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
