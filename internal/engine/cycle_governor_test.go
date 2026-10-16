package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewCycleGovernorDefaults(t *testing.T) {
	cases := []struct {
		name  string
		work  time.Duration
		rest  time.Duration
		wantW time.Duration
		wantR time.Duration
	}{
		{"both zero use defaults", 0, 0, DefaultWorkDuration, DefaultRestDuration},
		{"zero work uses default", 0, 7 * time.Minute, DefaultWorkDuration, 7 * time.Minute},
		{"zero rest uses default", 3 * time.Minute, 0, 3 * time.Minute, DefaultRestDuration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewCycleGovernor(tc.work, tc.rest)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g.Work != tc.wantW || g.Rest != tc.wantR {
				t.Errorf("got work=%s rest=%s, want %s/%s", g.Work, g.Rest, tc.wantW, tc.wantR)
			}
		})
	}
	if DefaultWorkDuration != 10*time.Minute || DefaultRestDuration != 5*time.Minute {
		t.Errorf("defaults drifted from spec §5: got %s/%s", DefaultWorkDuration, DefaultRestDuration)
	}
}

func TestNewCycleGovernorValidation(t *testing.T) {
	cases := []struct {
		name string
		work time.Duration
		rest time.Duration
	}{
		{"negative work", -time.Second, 5 * time.Minute},
		{"negative rest", 10 * time.Minute, -2 * time.Second},
		{"both negative", -time.Minute, -time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if g, err := NewCycleGovernor(tc.work, tc.rest); err == nil {
				t.Errorf("expected error for work=%s rest=%s, got governor %+v", tc.work, tc.rest, g)
			}
		})
	}
}

// TestCycleGovernorPhases walks accumulated dt across phase boundaries with
// Work=10s / Rest=5s: the rest begins exactly at elapsed==Work and a new work
// phase resumes exactly at each multiple of the 15s period.
func TestCycleGovernorPhases(t *testing.T) {
	g, err := NewCycleGovernor(10*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := []struct {
		dt        time.Duration
		wantAllow bool
		wantState State
		fragments []string
	}{
		{3 * time.Second, true, StateCopying, []string{"work phase: 7s remaining"}},
		{4 * time.Second, true, StateCopying, []string{"work phase: 3s remaining"}}, // elapsed 7s
		// boundary: exactly at Work the rest begins
		{3 * time.Second, false, StateResting, []string{"rest phase: 5s remaining until resume"}}, // elapsed 10s
		{2 * time.Second, false, StateResting, []string{"rest phase: 3s remaining until resume"}}, // elapsed 12s
		// boundary: exactly at the period a new work phase resumes
		{3 * time.Second, true, StateCopying, []string{"work phase: 10s remaining"}},              // elapsed 15s
		{8 * time.Second, true, StateCopying, []string{"work phase: 2s remaining"}},               // elapsed 23s → pos 8s
		{2 * time.Second, false, StateResting, []string{"rest phase: 5s remaining until resume"}}, // elapsed 25s → pos 10s
	}
	for i, s := range steps {
		t.Run("", func(t *testing.T) {
			allow, reason := g.Tick(s.dt)
			if allow != s.wantAllow {
				t.Errorf("step %d (dt=%s): allow = %v, want %v (reason %q)", i, s.dt, allow, s.wantAllow, reason)
			}
			if got := g.PauseState(); got != s.wantState {
				t.Errorf("step %d: PauseState = %s, want %s", i, got, s.wantState.String())
			}
			for _, frag := range s.fragments {
				if !strings.Contains(reason, frag) {
					t.Errorf("step %d: reason %q does not contain %q", i, reason, frag)
				}
			}
		})
	}
}

// TestCycleGovernorLargeDt verifies that a single dt larger than a full work+rest
// period lands in the correct phase of the right cycle (modulo arithmetic).
func TestCycleGovernorLargeDt(t *testing.T) {
	cases := []struct {
		name      string
		dt        time.Duration
		wantAllow bool
		fragments []string
	}{
		{"exactly one full period resumes work", 15 * time.Second, true, []string{"work phase: 10s remaining"}},
		{"37s lands mid-work of cycle two", 37 * time.Second, true, []string{"work phase: 3s remaining"}},                // 37 % 15 = 7 < 10
		{"42s lands in rest of cycle three", 42 * time.Second, false, []string{"rest phase: 3s remaining until resume"}}, // 42 % 15 = 12 >= 10
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewCycleGovernor(10*time.Second, 5*time.Second)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			allow, reason := g.Tick(tc.dt)
			if allow != tc.wantAllow {
				t.Errorf("allow = %v (reason %q), want %v", allow, reason, tc.wantAllow)
			}
			for _, frag := range tc.fragments {
				if !strings.Contains(reason, frag) {
					t.Errorf("reason %q does not contain %q", reason, frag)
				}
			}
		})
	}
}

func TestCycleGovernorCyclesForever(t *testing.T) {
	g, err := NewCycleGovernor(10*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Keep asking well past several periods: the governor must keep cycling
	// work/rest without terminating.
	for i := 0; i < 100; i++ {
		g.Tick(time.Second)
		pos := time.Duration(i+1) * time.Second % (g.Work + g.Rest)
		wantAllow := pos < g.Work
		if got := g.PauseState(); (got == StateResting) != !wantAllow {
			t.Fatalf("tick %d: PauseState = %s, want allow=%v", i+1, got, wantAllow)
		}
	}
}

func TestCycleGovernorNegativeDtIsClamped(t *testing.T) {
	g, err := NewCycleGovernor(10*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	allow, reason := g.Tick(-3 * time.Second)
	if !allow || !strings.Contains(reason, "work phase: 10s remaining") {
		t.Errorf("negative dt must not move the clock backwards: allow=%v reason=%q", allow, reason)
	}
	if got := g.PauseState(); got != StateCopying {
		t.Errorf("PauseState = %s after clamped tick, want copying", got)
	}
}
