package engine

import (
	"errors"
	"fmt"
	"time"
)

// Default sensor-mode thresholds in Celsius (spec §5).
const (
	DefaultTPauseC  = 50 // pause copying when temperature reaches this value
	DefaultTResumeC = 42 // resume copying when temperature falls to this value
)

// SensorGovernor pauses copying based on device temperature with hysteresis:
// once the temperature reaches TPause, copying stays paused until the
// temperature falls to TResume. The gap between the two thresholds prevents
// flapping when the sensor oscillates around a single boundary (spec FR-4).
//
// Threshold semantics (inclusive boundaries):
//   - temp >= TPause pauses copying;
//   - while paused, temp <= TResume resumes it.
//
// A failed reading (ok == false) never blocks: Tick allows the copy and
// resets the latch so the next valid reading re-evaluates from scratch.
type SensorGovernor struct {
	Src     TemperatureSource
	TPause  float64 // pause threshold, °C
	TResume float64 // resume threshold, °C; must be < TPause

	paused bool // latched: true once temp >= TPause until temp <= TResume
}

// NewSensorGovernor creates a SensorGovernor. A zero tPause or tResume selects
// the default (50 / 42 °C); negative values are rejected, as is any pair with
// tResume >= tPause (hysteresis requires a non-empty band).
func NewSensorGovernor(src TemperatureSource, tPause, tResume float64) (*SensorGovernor, error) {
	if src == nil {
		return nil, errors.New("sensor governor: temperature source is required")
	}
	if tPause < 0 || tResume < 0 {
		return nil, fmt.Errorf("sensor governor: thresholds must be non-negative (got T_pause %g, T_resume %g)", tPause, tResume)
	}
	if tPause == 0 {
		tPause = DefaultTPauseC
	}
	if tResume == 0 {
		tResume = DefaultTResumeC
	}
	if tResume >= tPause {
		return nil, fmt.Errorf("sensor governor: T_resume (%g°C) must be less than T_pause (%g°C)", tResume, tPause)
	}
	return &SensorGovernor{Src: src, TPause: tPause, TResume: tResume}, nil
}

// Tick reads the current temperature and reports whether copying may proceed.
// dt is accepted for Governor interface compatibility but ignored: the polling
// interval is the caller's concern (spec FR-4).
func (g *SensorGovernor) Tick(dt time.Duration) (allowCopy bool, reason string) {
	temp, ok := g.Src.Read()
	if !ok {
		g.paused = false // never block on missing data; re-evaluate next valid reading
		return true, "sensor unavailable"
	}

	switch {
	case !g.paused && temp >= g.TPause:
		g.paused = true
		return false, fmt.Sprintf("temp %.1f°C ≥ T_pause %.1f°C", temp, g.TPause)
	case g.paused && temp <= g.TResume:
		g.paused = false
		return true, fmt.Sprintf("temp %.1f°C ≤ T_resume %.1f°C", temp, g.TResume)
	case g.paused:
		return false, fmt.Sprintf("still paused: temp %.1f°C > T_resume %.1f°C", temp, g.TResume)
	default:
		return true, fmt.Sprintf("temp %.1f°C below T_pause %.1f°C", temp, g.TPause)
	}
}

// PauseState reports the engine state that applies right now: StatePausedThermal
// while the governor is blocking, StateCopying otherwise. It never returns a
// pause state for an allowed tick.
func (g *SensorGovernor) PauseState() State {
	if g.paused {
		return StatePausedThermal
	}
	return StateCopying
}
