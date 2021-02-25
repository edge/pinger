package pinger

import "time"

// Timer for a ping.
type Timer struct {
	Started time.Time
	Stopped time.Time
}

// Elapsed duration between the timer's start and stop times.
func (t *Timer) Elapsed() time.Duration {
	return t.Stopped.Sub(t.Started)
}

// Start timer.
func (t *Timer) Start() {
	t.Started = time.Now()
}

// Stop timer.
func (t *Timer) Stop() {
	t.Stopped = time.Now()
}
