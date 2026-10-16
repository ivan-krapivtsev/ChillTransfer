// Package engine implements the copy queue, recursive copier and thermal
// governor (T-002, T-003). It has no UI or platform dependencies.
package engine

import "fmt"

// State is a state of the copy process state machine.
type State int

const (
	StateIdle          State = iota // nothing in progress
	StateCopying                    // actively writing
	StatePausedThermal              // paused: temperature above T_pause
	StateResting                    // paused: cycle-mode rest phase
	StateDone                       // queue fully processed
	StateError                      // stopped due to error (option stop-on-error)
	StateCancelled                  // user cancelled the remaining queue
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateCopying:
		return "copying"
	case StatePausedThermal:
		return "paused_thermal"
	case StateResting:
		return "resting"
	case StateDone:
		return "done"
	case StateError:
		return "error"
	case StateCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Active reports whether the process is in a non-terminal state.
func (s State) Active() bool {
	switch s {
	case StateCopying, StatePausedThermal, StateResting:
		return true
	default:
		return false
	}
}
