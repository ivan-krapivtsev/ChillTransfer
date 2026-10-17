package ui

import (
	"fmt"
	"strconv"
	"strings"
)

// Copy-mode identifiers used by the mode radio group and validation helpers.
const (
	ModeSensor = "Датчик" // sensor mode: pause when temperature reaches T_pause
	ModeCycle  = "Цикл"   // cycle mode: fixed work/rest intervals
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
// must be positive. Defaults from SPEC §5 pass validation.
func ValidateSensorParams(tPauseC, tResumeC, pollSeconds float64) error {
	if tPauseC < MinTempC || tPauseC > MaxTempC {
		return fmt.Errorf("T_pause: %g вне диапазона %d…%d °C", tPauseC, MinTempC, MaxTempC)
	}
	if tResumeC < MinTempC || tResumeC > MaxTempC {
		return fmt.Errorf("T_resume: %g вне диапазона %d…%d °C", tResumeC, MinTempC, MaxTempC)
	}
	if tResumeC >= tPauseC {
		return fmt.Errorf("T_resume (%g) должен быть меньше T_pause (%g)", tResumeC, tPauseC)
	}
	if pollSeconds <= 0 {
		return fmt.Errorf("интервал опроса: %g с — должно быть больше нуля", pollSeconds)
	}
	return nil
}

// ValidateCycleParams checks cycle-mode inputs: work and rest durations must
// lie in [1..60] minutes. Defaults from SPEC §5 pass validation.
func ValidateCycleParams(workMinutes, restMinutes float64) error {
	if workMinutes < MinMinutes || workMinutes > MaxMinutes {
		return fmt.Errorf("рабочий интервал: %g мин вне диапазона %d…%d", workMinutes, MinMinutes, MaxMinutes)
	}
	if restMinutes < MinMinutes || restMinutes > MaxMinutes {
		return fmt.Errorf("отдых: %g мин вне диапазона %d…%d", restMinutes, MinMinutes, MaxMinutes)
	}
	return nil
}

// parseFloatParam parses a numeric mode-parameter entry value.
func parseFloatParam(text, name string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, fmt.Errorf("%s: некорректное число %q", name, text)
	}
	return v, nil
}
