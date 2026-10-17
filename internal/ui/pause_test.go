package ui_test

import (
	"testing"
	"time"

	"github.com/ivan-krapivtsev/chilltransfer/internal/engine"
	"github.com/ivan-krapivtsev/chilltransfer/internal/ui"
)

// TestManualPauseGate verifies the manual allow-flag wrapper: open gate allows,
// closed gate blocks and reports the mode's pause state.
func TestManualPauseGate(t *testing.T) {
	g := ui.NewManualPause(nil, engine.StateResting)
	if allow, _ := g.Tick(0); !allow {
		t.Error("open gate must allow copying")
	}
	if s := g.PauseState(); s != engine.StateCopying {
		t.Errorf("open gate PauseState = %v, want copying", s)
	}

	g.SetPaused(true)
	if !g.Paused() {
		t.Fatal("gate reports not paused after SetPaused(true)")
	}
	if allow, reason := g.Tick(0); allow || reason == "" {
		t.Errorf("closed gate: allow=%v reason=%q, want blocked with reason", allow, reason)
	}
	if s := g.PauseState(); s != engine.StateResting {
		t.Errorf("paused PauseState = %v, want resting", s)
	}

	g.SetPaused(false)
	if allow, _ := g.Tick(0); !allow {
		t.Error("reopened gate must allow copying")
	}
}

// TestManualPauseDelegatesToInnerGovernor verifies an open gate delegates to the
// inner governor and a closed gate blocks even in its work phase.
func TestManualPauseDelegatesToInnerGovernor(t *testing.T) {
	inner, err := engine.NewCycleGovernor(time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("NewCycleGovernor: %v", err)
	}
	g := ui.NewManualPause(inner, engine.StateResting)

	if allow, _ := g.Tick(0); !allow {
		t.Error("work phase through open gate must allow")
	}
	g.SetPaused(true)
	if allow, _ := g.Tick(time.Hour); allow {
		t.Error("closed gate must block even in the inner work phase")
	}
}
