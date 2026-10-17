package ui

import (
	"sync"
	"time"

	"github.com/ivan-krapivtsev/chilltransfer/internal/engine"
)

// ManualPause wraps an engine.Governor with a user-controlled allow flag — the
// T-005 design choice for FR-3 "Пауза/Продолжить". While the gate is closed,
// the copier's ensureAllowed loop blocks (re-checked every governor tick), so
// copying stops within one tick interval and resumes on the next allowed tick.
//
// While manually paused the wrapper reports pauseState as the engine state:
// sensor mode maps to StatePausedThermal, cycle mode to StateResting — both are
// "intentionally not copying" states in the SPEC §7 machine (the engine has no
// generic manual-pause state and T-005 must not modify it).
type ManualPause struct {
	inner      engine.Governor // may be nil: pure allow-flag gate
	pauseState engine.State    // reported while manually paused

	mu     sync.Mutex
	paused bool
}

// NewManualPause creates a pause gate around inner (nil allowed) that reports
// pauseState as the engine state while manually paused.
func NewManualPause(inner engine.Governor, pauseState engine.State) *ManualPause {
	return &ManualPause{inner: inner, pauseState: pauseState}
}

// SetPaused opens or closes the manual gate.
func (m *ManualPause) SetPaused(paused bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = paused
}

// Paused reports whether the manual gate is closed.
func (m *ManualPause) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// Tick implements engine.Governor: a closed gate blocks copying regardless of
// the inner governor; an open gate delegates to it (always allows when nil).
func (m *ManualPause) Tick(dt time.Duration) (bool, string) {
	if m.Paused() {
		return false, "manual pause"
	}
	if m.inner == nil {
		return true, ""
	}
	return m.inner.Tick(dt)
}

// PauseState implements engine.Governor: the manual state while paused,
// otherwise the inner governor's state (copying when there is no inner).
func (m *ManualPause) PauseState() engine.State {
	if m.Paused() {
		return m.pauseState
	}
	if m.inner != nil {
		return m.inner.PauseState()
	}
	return engine.StateCopying
}
