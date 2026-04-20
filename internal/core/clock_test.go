package core

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	c := RealClock{}
	before := time.Now().UnixMilli()
	got := c.UnixMilli()
	after := time.Now().UnixMilli()
	if got < before || got > after {
		t.Fatalf("RealClock.UnixMilli=%d out of [%d,%d]", got, before, after)
	}
	if c.Now().IsZero() {
		t.Fatal("RealClock.Now returned zero time")
	}
}

func TestSimClock_AdvanceAndSet(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewSimClock(start)

	if got := c.UnixMilli(); got != start.UnixMilli() {
		t.Fatalf("initial UnixMilli=%d want=%d", got, start.UnixMilli())
	}

	c.Advance(500 * time.Millisecond)
	if got := c.UnixMilli(); got != start.UnixMilli()+500 {
		t.Fatalf("after 500ms advance UnixMilli=%d want=%d", got, start.UnixMilli()+500)
	}

	// Negative advances must be ignored to preserve monotonicity.
	c.Advance(-time.Second)
	if got := c.UnixMilli(); got != start.UnixMilli()+500 {
		t.Fatalf("negative advance should be ignored, got=%d", got)
	}

	jump := start.Add(10 * time.Second)
	c.Set(jump)
	if got := c.Now(); !got.Equal(jump) {
		t.Fatalf("after Set, Now=%v want=%v", got, jump)
	}
}
