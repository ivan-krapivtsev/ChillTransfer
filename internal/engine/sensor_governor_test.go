package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// tempReading is one scripted reading of the fake temperature source.
type tempReading struct {
	tempC float64
	ok    bool
}

// fakeTempSource is a scripted TemperatureSource: Read returns readings in
// order and clamps to the last one once exhausted, so tests control every
// reading deterministically.
type fakeTempSource struct {
	readings []tempReading
	idx      int
}

func newFakeTempSource(temps ...float64) *fakeTempSource {
	readings := make([]tempReading, len(temps))
	for i, t := range temps {
		readings[i] = tempReading{tempC: t, ok: true}
	}
	return &fakeTempSource{readings: readings}
}

func (f *fakeTempSource) Read() (float64, bool) {
	r := f.readings[f.idx]
	if f.idx < len(f.readings)-1 {
		f.idx++
	}
	return r.tempC, r.ok
}

// tickSensor advances the governor by one scripted reading and checks the
// allow flag, pause state and reason fragments.
func tickSensor(t *testing.T, g *SensorGovernor, wantAllow bool, wantState State, wantFragments []string) {
	t.Helper()
	allow, reason := g.Tick(3 * time.Second) // dt is ignored by the sensor governor
	if allow != wantAllow {
		t.Errorf("Tick: allow = %v, want %v (reason %q)", allow, wantAllow, reason)
	}
	if got := g.PauseState(); got != wantState {
		t.Errorf("PauseState = %s, want %s", got, wantState.String())
	}
	for _, frag := range wantFragments {
		if !strings.Contains(reason, frag) {
			t.Errorf("reason %q does not contain %q", reason, frag)
		}
	}
}

func TestNewSensorGovernorDefaults(t *testing.T) {
	cases := []struct {
		name    string
		tPause  float64
		tResume float64
		wantP   float64
		wantR   float64
	}{
		{"both zero use defaults", 0, 0, DefaultTPauseC, DefaultTResumeC},
		{"zero T_pause uses default", 0, 40, DefaultTPauseC, 40},
		{"zero T_resume uses default", 60, 0, 60, DefaultTResumeC},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewSensorGovernor(newFakeTempSource(), tc.tPause, tc.tResume)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g.TPause != tc.wantP || g.TResume != tc.wantR {
				t.Errorf("got T_pause=%g T_resume=%g, want %g/%g", g.TPause, g.TResume, tc.wantP, tc.wantR)
			}
		})
	}
	if float64(DefaultTPauseC) != 50 || float64(DefaultTResumeC) != 42 {
		t.Errorf("defaults drifted from spec §5: got %v/%v", DefaultTPauseC, DefaultTResumeC)
	}
}

func TestNewSensorGovernorValidation(t *testing.T) {
	cases := []struct {
		name    string
		src     TemperatureSource
		tPause  float64
		tResume float64
	}{
		{"nil source", nil, 50, 42},
		{"negative T_pause", newFakeTempSource(), -1, 42},
		{"negative T_resume", newFakeTempSource(), 50, -3},
		{"T_resume equals T_pause", newFakeTempSource(), 50, 50},
		{"T_resume above T_pause", newFakeTempSource(), 40, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if g, err := NewSensorGovernor(tc.src, tc.tPause, tc.tResume); err == nil {
				t.Errorf("expected error for T_pause=%g T_resume=%g, got governor %+v", tc.tPause, tc.tResume, g)
			}
		})
	}
}

// TestSensorGovernorHysteresis walks a scripted temperature trace through the
// full latch cycle with thresholds 50/42: pause at TPause (inclusive), stay
// latched inside the hysteresis band, resume only at TResume (inclusive).
func TestSensorGovernorHysteresis(t *testing.T) {
	src := newFakeTempSource(45, 50, 51, 49, 48, 42, 41)
	g, err := NewSensorGovernor(src, 50, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := []struct {
		temp      float64
		wantAllow bool
		wantState State
		fragments []string
	}{
		{45, true, StateCopying, []string{"temp 45.0°C below T_pause 50.0°C"}},
		// boundary: exactly at TPause pauses (>= semantics)
		{50, false, StatePausedThermal, []string{"temp 50.0°C ≥ T_pause 50.0°C"}},
		{51, false, StatePausedThermal, []string{"still paused", "temp 51.0°C > T_resume 42.0°C"}},
		// inside the hysteresis band: latched, no resume above TResume
		{49, false, StatePausedThermal, []string{"still paused"}},
		{48, false, StatePausedThermal, []string{"still paused"}},
		// boundary: exactly at TResume resumes (<= semantics)
		{42, true, StateCopying, []string{"temp 42.0°C ≤ T_resume 42.0°C"}},
		{41, true, StateCopying, []string{"below T_pause"}},
	}
	for i, s := range steps {
		t.Run(fmt.Sprintf("tick %d (temp %.1f°C)", i, s.temp), func(t *testing.T) {
			tickSensor(t, g, s.wantAllow, s.wantState, s.fragments)
		})
	}
}

// TestSensorGovernorBoundaries pins the inclusive boundary semantics: >= pauses,
// <= resumes; one step inside either side does not trigger a transition.
func TestSensorGovernorBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		pre       []float64 // readings that set up the latch state first
		at        float64   // reading under test
		wantAllow bool
	}{
		{"exactly TPause pauses", nil, 50, false},
		{"just below TPause allows", nil, 49.9, true},
		{"just above TPause pauses", nil, 50.1, false},
		{"exactly TResume resumes when latched", []float64{52}, 42, true},
		{"just above TResume stays paused when latched", []float64{52}, 42.1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := newFakeTempSource(append(tc.pre, tc.at)...)
			g, err := NewSensorGovernor(src, 50, 42)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for range tc.pre {
				g.Tick(time.Second) // consume setup readings
			}
			allow, reason := g.Tick(time.Second)
			if allow != tc.wantAllow {
				t.Errorf("allow = %v (reason %q), want %v", allow, reason, tc.wantAllow)
			}
		})
	}
}

// TestSensorGovernorNoFlapping verifies that oscillation inside the hysteresis
// band never toggles the state: one pause and exactly one resume for the whole
// trace, with no re-latch below TPause after resuming.
func TestSensorGovernorNoFlapping(t *testing.T) {
	src := newFakeTempSource(50, 43, 49, 43, 49, 43, 42, 49, 41, 49)
	g, err := NewSensorGovernor(src, 50, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantAllow := []bool{false, false, false, false, false, false, true, true, true, true}
	transitions := 0
	var prev bool
	for i, want := range wantAllow {
		allow, reason := g.Tick(time.Second)
		if allow != want {
			t.Fatalf("tick %d: allow = %v (reason %q), want %v", i, allow, reason, want)
		}
		if i > 0 && allow != prev {
			transitions++
		}
		prev = allow
	}
	if transitions != 1 {
		t.Errorf("state transitions = %d, want exactly 1 (pause→resume)", transitions)
	}
}

// TestSensorGovernorUnavailable verifies that missing readings never block:
// every failed read allows the copy with reason "sensor unavailable" and resets
// the latch so a later valid reading re-evaluates from scratch.
func TestSensorGovernorUnavailable(t *testing.T) {
	src := &fakeTempSource{readings: []tempReading{
		{52, true}, // latches paused
		{0, false}, // sensor drops out while hot
		{0, false}, // still down
		{40, true}, // back with a cool reading
	}}
	g, err := NewSensorGovernor(src, 50, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, reason := g.Tick(time.Second)
	if allow || !strings.Contains(reason, "temp 52.0°C ≥ T_pause") {
		t.Errorf("hot reading: allow=%v reason=%q, want blocked at T_pause", allow, reason)
	}

	for i := 1; i <= 2; i++ { // the two ok=false readings
		allow, reason = g.Tick(time.Second)
		if !allow {
			t.Fatalf("tick %d: failed reading must not block (reason %q)", i, reason)
		}
		if got := g.PauseState(); got != StateCopying {
			t.Errorf("tick %d: PauseState = %s while allowed, want copying", i, got)
		}
	}
	if reason != "sensor unavailable" {
		t.Errorf(`reason = %q, want exactly "sensor unavailable"`, reason)
	}

	// Latch was reset by the failed readings: a cool reading stays allowed.
	allow, _ = g.Tick(time.Second)
	if !allow {
		t.Error("valid cool reading after outage should be allowed")
	}
}

func TestSensorGovernorUnavailableAlwaysAllowed(t *testing.T) {
	src := &fakeTempSource{readings: []tempReading{{0, false}, {99, false}}}
	g, err := NewSensorGovernor(src, 50, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 3; i++ { // third read clamps to the last scripted one
		allow, reason := g.Tick(time.Second)
		if !allow || reason != "sensor unavailable" {
			t.Errorf("tick %d: allow=%v reason=%q, want allowed with 'sensor unavailable'", i, allow, reason)
		}
	}
}
