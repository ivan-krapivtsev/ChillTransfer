package engine

import "time"

// TemperatureSource provides best-effort temperature readings for a device.
type TemperatureSource interface {
	// Read returns the current temperature in Celsius. ok is false when no
	// reading is available (no sensor, permission denied, etc.).
	Read() (tempC float64, ok bool)
}

// Governor decides on each tick whether copying may proceed. Implementations:
// SensorGovernor and CycleGovernor (T-003).
type Governor interface {
	// Tick advances the governor by dt and reports whether copying is allowed.
	Tick(dt time.Duration) (allowCopy bool, reason string)
}
