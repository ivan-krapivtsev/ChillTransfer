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
// injected-provider coverage. NewWindow detects the language from locale env,
// so the expected label is resolved with the same detection.
func TestNewWindowSmoke(t *testing.T) {
	w := ui.NewWindow(test.NewApp())
	if w == nil || w.Content() == nil {
		t.Fatal("window or content is nil")
	}
	want := ui.NewI18N(ui.DetectLanguage()).T(ui.KeyDropZone)
	if !containsLabel(w.Content(), want) {
		t.Fatalf("drop zone label %q not found in window content", want)
	}
}

// TestContentLayoutContainsRequiredSections verifies the SPEC §6 layout: device
// panel on top, drop zone + queue in the middle, mode settings and controls at
// the bottom. The UI is RU-pinned (see newTestUI), so expected texts come from
// the same I18N instance.
func TestContentLayoutContainsRequiredSections(t *testing.T) {
	u := newTestUI(t, &fakeLister{})
	root := u.Content()
	i := u.I18N

	for _, key := range []string{ui.KeyDropZone, ui.KeyDeviceLabel, ui.KeyQueueLabel, ui.KeyModeLabel} {
		if !containsLabel(root, i.T(key)) {
			t.Errorf("label %q not found in content", i.T(key))
		}
	}
	if u.StartButton.Text != i.T(ui.KeyBtnStart) || u.PauseButton.Text != i.T(ui.KeyBtnPause) || u.StopButton.Text != i.T(ui.KeyBtnStop) {
		t.Errorf("control labels = %q/%q/%q, want %q/%q/%q",
			u.StartButton.Text, u.PauseButton.Text, u.StopButton.Text,
			i.T(ui.KeyBtnStart), i.T(ui.KeyBtnPause), i.T(ui.KeyBtnStop))
	}
	for _, opt := range []string{i.T(ui.KeyModeSensor), i.T(ui.KeyModeCycle)} {
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

// TestNewWithI18NEnglishRendersEnglishLabels verifies the EN table drives
// user-visible widget text (T-007): run controls, drop zone hint and device
// caption render in natural English when the UI is built with LangEN.
func TestNewWithI18NEnglishRendersEnglishLabels(t *testing.T) {
	u := ui.NewWithI18N(test.NewApp(), &fakeLister{}, ui.NewI18N(ui.LangEN))

	if u.StartButton.Text != "Start" || u.PauseButton.Text != "Pause" || u.StopButton.Text != "Stop" {
		t.Errorf("control labels = %q/%q/%q, want Start/Pause/Stop", u.StartButton.Text, u.PauseButton.Text, u.StopButton.Text)
	}
	if !containsLabel(u.Content(), "Drop files and folders here") {
		t.Error("drop zone hint not rendered in English")
	}
	if !containsLabel(u.Content(), "Device:") {
		t.Error("device caption not rendered in English")
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
