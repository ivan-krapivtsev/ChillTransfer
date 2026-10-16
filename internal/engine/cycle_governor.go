package engine

import (
	"fmt"
	"time"
)

// Default work/rest durations for cycle mode (spec §5).
const (
	DefaultWorkDuration = 10 * time.Minute
	DefaultRestDuration = 5 * time.Minute
)

// CycleGovernor alternates a work phase of length Work and a rest phase of
// length Rest, cycling forever until the copier stops calling Tick (spec FR-5).
// It is fully deterministic: an internal elapsed counter advances by dt on each
// Tick; no wall clock is used.
//
// Phase boundaries for period P = Work + Rest are half-open intervals within
// each cycle k·P … (k+1)·P:
//   - [k·P, k·P+Work) — work phase, copying allowed;
//   - [k·P+Work, (k+1)·P) — rest phase, copying blocked.
//
// So the rest begins exactly at elapsed == Work and a new work phase resumes
// exactly at each multiple of P.
type CycleGovernor struct {
	Work time.Duration // copy duration per cycle
	Rest time.Duration // pause duration per cycle

	elapsed time.Duration // accumulated dt across all Ticks since construction
}

// NewCycleGovernor creates a CycleGovernor. A zero work or rest selects the
// default (10 / 5 minutes); negative values are rejected.
func NewCycleGovernor(work, rest time.Duration) (*CycleGovernor, error) {
	if work < 0 || rest < 0 {
		return nil, fmt.Errorf("cycle governor: durations must be non-negative (got work %s, rest %s)", work, rest)
	}
	if work == 0 {
		work = DefaultWorkDuration
	}
	if rest == 0 {
		rest = DefaultRestDuration
	}
	return &CycleGovernor{Work: work, Rest: rest}, nil
}

// Tick advances the internal elapsed counter by dt and reports whether copying
// is allowed in the resulting phase. A negative dt is clamped to zero.
func (g *CycleGovernor) Tick(dt time.Duration) (allowCopy bool, reason string) {
	if dt < 0 {
		dt = 0
	}
	g.elapsed += dt

	pos := g.elapsed % (g.Work + g.Rest)
	if pos < g.Work {
		return true, fmt.Sprintf("work phase: %s remaining", g.Work-pos)
	}
	return false, fmt.Sprintf("rest phase: %s remaining until resume", g.Work+g.Rest-pos)
}

// PauseState reports the engine state that applies right now: StateResting while
// the governor is blocking, StateCopying otherwise. It never returns a pause
// state for an allowed tick.
func (g *CycleGovernor) PauseState() State {
	if g.elapsed%(g.Work+g.Rest) >= g.Work {
		return StateResting
	}
	return StateCopying
}
