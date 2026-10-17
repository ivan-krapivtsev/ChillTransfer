package ui

import (
	"fmt"
	"strconv"
	"strings"
)

// Copy-mode identifiers — stable, language-neutral values used by the mode
// radio group's selection logic and validation helpers. User-visible labels
// come from I18N (KeyModeSensor/KeyModeCycle); see UI.modeLabel/selectedMode.
const (
	ModeSensor = "sensor" // sensor mode: pause when temperature reaches T_pause
	ModeCycle  = "cycle"  // cycle mode: fixed work/rest intervals
)

// DefaultPollSeconds is the default sensor polling interval in seconds (SPEC §5).
const DefaultPollSeconds = 3

// Allowed input ranges (SPEC §5).
const (
	MinTempC   = 30 // minimum allowed temperature threshold, °C
	MaxTempC   = 80 // maximum allowed temperature threshold, °C
	MinMinutes = 1  // minimum work/rest duration, minutes
	MaxMinutes = 60 // maximum work/rest duration, minutes
)

// ValidateSensorParams checks sensor-mode inputs: both thresholds must lie in
// [30..80] °C with T_resume < T_pause (hysteresis), and the poll interval
// must be positive. Defaults from SPEC §5 pass validation. Errors are rendered
// via i (T-007).
func ValidateSensorParams(i *I18N, tPauseC, tResumeC, pollSeconds float64) error {
	if tPauseC < MinTempC || tPauseC > MaxTempC {
		return fmt.Errorf(i.T(KeyErrTPauseRange), tPauseC, MinTempC, MaxTempC)
	}
	if tResumeC < MinTempC || tResumeC > MaxTempC {
		return fmt.Errorf(i.T(KeyErrTResumeRange), tResumeC, MinTempC, MaxTempC)
	}
	if tResumeC >= tPauseC {
		return fmt.Errorf(i.T(KeyErrHysteresis), tResumeC, tPauseC)
	}
	if pollSeconds <= 0 {
		return fmt.Errorf(i.T(KeyErrPollPositive), pollSeconds)
	}
	return nil
}

// ValidateCycleParams checks cycle-mode inputs: work and rest durations must
// lie in [1..60] minutes. Defaults from SPEC §5 pass validation. Errors are
// rendered via i (T-007).
func ValidateCycleParams(i *I18N, workMinutes, restMinutes float64) error {
	if workMinutes < MinMinutes || workMinutes > MaxMinutes {
		return fmt.Errorf(i.T(KeyErrWorkRange), workMinutes, MinMinutes, MaxMinutes)
	}
	if restMinutes < MinMinutes || restMinutes > MaxMinutes {
		return fmt.Errorf(i.T(KeyErrRestRange), restMinutes, MinMinutes, MaxMinutes)
	}
	return nil
}

// parseFloatParam parses a numeric mode-parameter entry value; name is the
// localized parameter caption used in the error message.
func parseFloatParam(i *I18N, text, name string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, fmt.Errorf(i.T(KeyErrBadNumber), name, text)
	}
	return v, nil
}
