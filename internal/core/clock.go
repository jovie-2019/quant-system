// Package core provides transport-agnostic abstractions shared by live trading
// and backtesting: a trading Engine that orchestrates risk → execution → FSM →
// ledger, a Clock for wall-clock vs simulated time, and an EventSink for
// publishing lifecycle events (NATS in live, in-memory in backtest).
package core

import (
	"sync/atomic"
	"time"
)

// Clock abstracts time so backtests can replay historical data under a
// simulated clock while live trading uses the wall clock.
type Clock interface {
	Now() time.Time
	UnixMilli() int64
}

// RealClock returns wall-clock time.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// UnixMilli returns the current wall-clock time in milliseconds since epoch.
func (RealClock) UnixMilli() int64 { return time.Now().UnixMilli() }

// SimClock is a monotonic simulated clock driven by explicit Advance / Set
// calls. Safe for concurrent reads; writers should synchronise externally when
// strict ordering is required.
type SimClock struct {
	nanos atomic.Int64
}

// NewSimClock returns a SimClock initialised to the given time.
func NewSimClock(start time.Time) *SimClock {
	c := &SimClock{}
	c.nanos.Store(start.UnixNano())
	return c
}

// Now returns the current simulated time.
func (c *SimClock) Now() time.Time { return time.Unix(0, c.nanos.Load()) }

// UnixMilli returns the current simulated time in milliseconds since epoch.
func (c *SimClock) UnixMilli() int64 { return c.nanos.Load() / int64(time.Millisecond) }

// Advance moves the simulated clock forward by d. Negative values are ignored
// to preserve monotonicity.
func (c *SimClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.nanos.Add(int64(d))
}

// Set jumps the clock to t. Callers are responsible for ensuring t is not
// earlier than the current value when monotonicity matters.
func (c *SimClock) Set(t time.Time) { c.nanos.Store(t.UnixNano()) }
