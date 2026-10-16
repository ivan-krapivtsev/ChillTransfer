package engine

import "time"

// Clock abstracts time so governor and copier logic are deterministic in tests.
type Clock interface {
	Now() time.Time
}

// RealClock is the production clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
