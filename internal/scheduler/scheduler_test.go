package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock drives a fakeTicker; NewTicker returns the same fakeTicker
// so tests can Fire() it deterministically.
type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	tickers  []*fakeTicker
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (c *fakeClock) NewTicker(_ time.Duration) Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTicker{ch: make(chan time.Time, 8)}
	c.tickers = append(c.tickers, t)
	return t
}

// FireAll advances every active fakeTicker once with the current clock.
func (c *fakeClock) FireAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.tickers {
		t.ch <- c.now
	}
}

// WaitForTickers blocks until at least n tickers have been registered
// with this clock. This is the test-side analogue of "wait for all the
// scheduler goroutines to reach their NewTicker call"; without it,
// FireAll can silently race the Start() goroutines and miss some of
// them entirely.
func (c *fakeClock) WaitForTickers(t *testing.T, n int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		have := len(c.tickers)
		c.mu.Unlock()
		if have >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("only %d tickers registered within %s (want >= %d)", len(c.tickers), within, n)
}

type fakeTicker struct {
	ch      chan time.Time
	stopped atomic.Bool
}

func (t *fakeTicker) C() <-chan time.Time { return t.ch }
func (t *fakeTicker) Stop()                { t.stopped.Store(true) }

// countJob increments a counter every time Run fires.
type countJob struct {
	name  string
	count atomic.Int32
	err   error
	panic bool
}

func (j *countJob) Name() string { return j.name }
func (j *countJob) Run(_ context.Context) error {
	if j.panic {
		panic("boom")
	}
	j.count.Add(1)
	return j.err
}

func TestScheduler_RunImmediately(t *testing.T) {
	clock := newFakeClock(time.Now())
	sch := New(Config{Clock: clock})
	j := &countJob{name: "immediate"}
	sch.Register(j, 100*time.Millisecond, true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sch.Start(ctx)
		close(done)
	}()

	// RunImmediately should fire at least once without any ticker firing.
	waitFor(t, func() bool { return j.count.Load() >= 1 }, time.Second)

	cancel()
	<-done
}

func TestScheduler_FiresOnTicker(t *testing.T) {
	clock := newFakeClock(time.Now())
	sch := New(Config{Clock: clock})
	j := &countJob{name: "ticking"}
	sch.Register(j, time.Second, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sch.Start(ctx)
		close(done)
	}()

	// Wait for the run-loop goroutine to register its ticker, otherwise
	// FireAll() would race the scheduler and the tick could be missed.
	clock.WaitForTickers(t, 1, time.Second)
	if j.count.Load() != 0 {
		t.Fatalf("count=%d want 0 before first tick", j.count.Load())
	}

	clock.FireAll()
	waitFor(t, func() bool { return j.count.Load() >= 1 }, time.Second)

	clock.FireAll()
	clock.FireAll()
	waitFor(t, func() bool { return j.count.Load() >= 3 }, time.Second)

	cancel()
	<-done
}

func TestScheduler_IsolatesJobs(t *testing.T) {
	clock := newFakeClock(time.Now())
	sch := New(Config{Clock: clock})
	a := &countJob{name: "a"}
	b := &countJob{name: "b", err: errors.New("bad")}
	c := &countJob{name: "c", panic: true}
	sch.Register(a, time.Second, false)
	sch.Register(b, time.Second, false)
	sch.Register(c, time.Second, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sch.Start(ctx)
		close(done)
	}()

	// Wait for all three run-loop goroutines to register their tickers
	// before firing, otherwise the first FireAll can race the
	// goroutines into existence and miss them.
	clock.WaitForTickers(t, 3, time.Second)

	clock.FireAll()
	waitFor(t, func() bool { return a.count.Load() >= 1 }, time.Second)

	clock.FireAll()
	waitFor(t, func() bool { return a.count.Load() >= 2 }, time.Second)

	// b is the error-returning job; it still increments before returning.
	if b.count.Load() < 1 {
		t.Fatalf("b should run despite returning an error: %d", b.count.Load())
	}

	cancel()
	<-done
}

func TestScheduler_CancelStopsAllJobs(t *testing.T) {
	clock := newFakeClock(time.Now())
	sch := New(Config{Clock: clock})
	j := &countJob{name: "x"}
	sch.Register(j, time.Second, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sch.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not return after ctx cancel")
	}
}

func TestJobFunc_RunDelegates(t *testing.T) {
	var called atomic.Bool
	j := JobFunc{
		N: "wrapper",
		F: func(_ context.Context) error {
			called.Store(true)
			return nil
		},
	}
	if err := j.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Fatal("captured function was not called")
	}
	if j.Name() != "wrapper" {
		t.Fatalf("name=%s", j.Name())
	}
}

// waitFor polls cond until true or timeout.
func waitFor(t *testing.T, cond func() bool, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}
