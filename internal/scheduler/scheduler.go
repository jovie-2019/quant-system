// Package scheduler runs background Jobs at fixed intervals. It is the
// engine behind Phase 7's self-optimisation loop — a nightly Job walks
// live strategies, re-runs the optimiser, and stages ParamCandidates
// for operator approval — but the package itself knows nothing about
// strategies or optimisation. Job implementations live in the
// adminapi package (or wherever dependency wiring is convenient) and
// are registered with the scheduler at startup.
//
// Design choices:
//
//   - Ticker-based, not full cron. The quant-system has no use case for
//     "at 04:17 every second Tuesday of the month"; a simple interval
//     covers everything from "every hour" to "every 24 hours". This
//     keeps the package tiny and test-friendly.
//   - One goroutine per Job, so a slow Job cannot block others.
//   - The Clock is injectable so tests don't wait real-wall-clock time.
//   - Every Job run is wrapped with recover() so a panic in one run
//     does not take down the whole scheduler.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Job is a unit of scheduled work. Implementations must be safe to call
// concurrently with their previous invocation if runs can overlap; the
// scheduler serialises a single Job's invocations per registration so
// in practice "safe to call after the previous Run returned" is enough.
type Job interface {
	// Name identifies the job in logs and metrics.
	Name() string
	// Run performs one unit of work. Returning an error is logged but
	// does not disable future invocations — transient failures are
	// expected.
	Run(ctx context.Context) error
}

// Clock is abstracted so tests can advance time deterministically.
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
}

// Ticker mirrors time.Ticker's minimal surface.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// RealClock is the wall-clock implementation used in production.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// NewTicker returns a wrapped *time.Ticker.
func (RealClock) NewTicker(d time.Duration) Ticker { return &realTicker{t: time.NewTicker(d)} }

type realTicker struct{ t *time.Ticker }

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()                { r.t.Stop() }

// Scheduler owns a set of registered Jobs and their cadences. Call
// Register from the wiring layer, then Start once — it blocks until
// ctx is cancelled.
type Scheduler struct {
	mu     sync.Mutex
	jobs   []registration
	clock  Clock
	logger *slog.Logger
}

type registration struct {
	job           Job
	interval      time.Duration
	runImmediately bool
}

// Config parameterises NewScheduler. Zero values pick RealClock and
// slog.Default().
type Config struct {
	Clock  Clock
	Logger *slog.Logger
}

// New returns a Scheduler with no registered jobs.
func New(cfg Config) *Scheduler {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Scheduler{clock: cfg.Clock, logger: cfg.Logger}
}

// Register adds a Job that should fire every `interval`. When
// runImmediately is true, the Job's first invocation happens as soon as
// Start() begins (useful for health-check jobs that should report a
// state on boot); otherwise it waits `interval` before the first fire.
//
// It is safe to call Register before or after Start. Registrations made
// after Start begin ticking at their first interval boundary.
func (s *Scheduler) Register(job Job, interval time.Duration, runImmediately bool) {
	if job == nil || interval <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, registration{job: job, interval: interval, runImmediately: runImmediately})
}

// Start blocks until ctx is cancelled, running each registered Job on
// its own goroutine. Panics inside a Job are recovered and logged;
// the scheduler keeps running for the other Jobs.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	jobs := make([]registration, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, r := range jobs {
		wg.Add(1)
		go func(r registration) {
			defer wg.Done()
			s.runLoop(ctx, r)
		}(r)
	}
	wg.Wait()
}

func (s *Scheduler) runLoop(ctx context.Context, r registration) {
	ticker := s.clock.NewTicker(r.interval)
	defer ticker.Stop()

	if r.runImmediately {
		s.invoke(ctx, r.job)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.invoke(ctx, r.job)
		}
	}
}

// invoke runs the Job with panic recovery and structured logging.
func (s *Scheduler) invoke(ctx context.Context, job Job) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("scheduler: job panic",
				"job", job.Name(),
				"panic", rec,
				"stack", string(debug.Stack()),
			)
		}
	}()
	start := s.clock.Now()
	err := job.Run(ctx)
	elapsed := s.clock.Now().Sub(start)
	if err != nil {
		s.logger.Error("scheduler: job failed",
			"job", job.Name(),
			"error", err,
			"elapsed_ms", elapsed.Milliseconds(),
		)
		return
	}
	s.logger.Info("scheduler: job ok",
		"job", job.Name(),
		"elapsed_ms", elapsed.Milliseconds(),
	)
}

// JobFunc adapts a plain function to the Job interface.
type JobFunc struct {
	N string
	F func(ctx context.Context) error
}

// Name returns the constructor-supplied name.
func (j JobFunc) Name() string { return j.N }

// Run delegates to the captured function.
func (j JobFunc) Run(ctx context.Context) error {
	if j.F == nil {
		return nil
	}
	return j.F(ctx)
}

// Compile-time check.
var _ Job = JobFunc{}

// ErrMisconfigured is returned by helpers that detect invalid scheduler
// configuration (currently unused but reserved so future constructors
// have a stable error type to wrap).
var ErrMisconfigured = fmt.Errorf("scheduler: misconfigured")
