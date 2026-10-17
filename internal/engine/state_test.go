package engine

import (
	"testing"
)

func TestStateStringsAreUniqueAndNonEmpty(t *testing.T) {
	states := []State{
		StateIdle, StateCopying, StatePausedThermal, StateResting,
		StateDone, StateError, StateCancelled,
	}
	seen := map[string]bool{}
	for _, s := range states {
		name := s.String()
		if name == "" || name == "unknown" {
			t.Errorf("state %d has empty/unknown string", int(s))
		}
		if seen[name] {
			t.Errorf("duplicate state string %q", name)
		}
		seen[name] = true
	}
}

func TestStateActive(t *testing.T) {
	active := []State{StateCopying, StatePausedThermal, StateResting}
	for _, s := range active {
		if !s.Active() {
			t.Errorf("%s should be active", s.String())
		}
	}
	inactive := []State{StateIdle, StateDone, StateError, StateCancelled}
	for _, s := range inactive {
		if s.Active() {
			t.Errorf("%s should not be active", s.String())
		}
	}
}
