package ui_test

import (
	"testing"

	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

// TestValidateSensorParams verifies SPEC §5 rules: defaults pass, thresholds are
// bounded to 30..80 °C with T_resume < T_pause, poll interval must be positive.
func TestValidateSensorParams(t *testing.T) {
	valid := []struct {
		name     string
		tPause   float64
		tResume  float64
		pollSecs float64
	}{
		{"defaults", 50, 42, 3},
		{"boundary low", 31, 30, 1},
		{"boundary high", 80, 79, 60},
	}
	for _, tc := range valid {
		if err := ui.ValidateSensorParams(tc.tPause, tc.tResume, tc.pollSecs); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	invalid := []struct {
		name     string
		tPause   float64
		tResume  float64
		pollSecs float64
	}{
		{"tpause below range", 29.9, 25, 3},
		{"tpause above range", 80.1, 70, 3},
		{"tresume below range", 40, 29.9, 3},
		{"tresume above range", 60, 80.1, 3},
		{"tresume equals tpause", 50, 50, 3},
		{"tresume above tpause", 42, 50, 3},
		{"poll zero", 50, 42, 0},
		{"poll negative", 50, 42, -1},
	}
	for _, tc := range invalid {
		if err := ui.ValidateSensorParams(tc.tPause, tc.tResume, tc.pollSecs); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

// TestValidateCycleParams verifies work/rest durations are bounded to 1..60 minutes.
func TestValidateCycleParams(t *testing.T) {
	valid := []struct {
		name    string
		workMin float64
		restMin float64
	}{
		{"defaults", 10, 5},
		{"boundary low", 1, 1},
		{"boundary high", 60, 60},
	}
	for _, tc := range valid {
		if err := ui.ValidateCycleParams(tc.workMin, tc.restMin); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	invalid := []struct {
		name    string
		workMin float64
		restMin float64
	}{
		{"work zero", 0, 5},
		{"work above range", 61, 5},
		{"work negative", -1, 5},
		{"rest zero", 10, 0},
		{"rest above range", 10, 61},
	}
	for _, tc := range invalid {
		if err := ui.ValidateCycleParams(tc.workMin, tc.restMin); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}
