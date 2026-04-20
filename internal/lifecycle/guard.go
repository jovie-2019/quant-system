package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// EvidenceSource abstracts the data a promotion Guard inspects. The
// caller (typically adminapi) wires this to the backtest store, trade
// history, and shadow-mode log. Leaving it as an interface keeps the
// guard logic testable with synthetic evidence.
type EvidenceSource interface {
	// BestBacktestSharpe returns the Sharpe of the best (by
	// objective) completed backtest for this strategy that referenced
	// the currently-deployed parameter set. Zero or NaN means "no
	// trusted backtest".
	BestBacktestSharpe(ctx context.Context, strategyID string) (float64, error)

	// ShadowRunDuration returns how long the strategy has been
	// running with shadow_on. Used as a time-based gate for
	// paper → canary.
	ShadowRunDuration(ctx context.Context, strategyID string) (time.Duration, error)

	// ShadowVirtualPnL returns the unrealised PnL accumulated by
	// shadow-mode intents over the ShadowRunDuration window.
	// Interpretation is strategy-agnostic — positive is good.
	ShadowVirtualPnL(ctx context.Context, strategyID string) (float64, error)

	// CanaryRunDuration / CanaryLiveSharpe mirror the shadow pair but
	// source from the live execution path with size capped to the
	// canary budget.
	CanaryRunDuration(ctx context.Context, strategyID string) (time.Duration, error)
	CanaryLiveSharpe(ctx context.Context, strategyID string) (float64, error)
}

// Policy groups the numeric thresholds a Guard compares against. These
// are intentionally conservative out of the box — operators tune them
// per-strategy and store the overrides alongside the strategy config.
type Policy struct {
	// MinBacktestSharpe is the minimum Sharpe a backtest must produce
	// before the strategy can be promoted out of draft.
	MinBacktestSharpe float64

	// MinShadowDuration is how long a strategy must stay in paper
	// before being eligible for canary promotion. 24h default.
	MinShadowDuration time.Duration

	// MinShadowVirtualPnL is the minimum cumulative virtual PnL
	// observed during shadow; zero means "any non-negative outcome".
	MinShadowVirtualPnL float64

	// MinCanaryDuration is how long a strategy must stay in canary
	// before full live promotion. 72h default — long enough to see a
	// few market regimes on a 1m-bar strategy.
	MinCanaryDuration time.Duration

	// MaxSharpeDrift is the maximum allowed drop of canary live
	// Sharpe vs backtest Sharpe. Expressed as an absolute difference.
	// A canary at Sharpe 1.0 with backtest 2.0 has a drift of 1.0.
	MaxSharpeDrift float64
}

// DefaultPolicy returns the out-of-the-box conservative thresholds.
func DefaultPolicy() Policy {
	return Policy{
		MinBacktestSharpe:   0.8,
		MinShadowDuration:   24 * time.Hour,
		MinShadowVirtualPnL: 0,
		MinCanaryDuration:   72 * time.Hour,
		MaxSharpeDrift:      0.6,
	}
}

// Check evaluates whether the requested forward promotion is allowed
// given the current evidence. It returns nil on success or a descriptive
// error wrapping ErrGuardFailed on rejection. Demotions and
// kill-switches skip Check — callers dispatch by TransitionKind.
func Check(ctx context.Context, kind TransitionKind, strategyID string, from, to Stage, src EvidenceSource, pol Policy) error {
	if kind != TransitionPromote {
		return nil
	}
	if src == nil {
		return fmt.Errorf("%w: evidence source is required for promotion", ErrGuardFailed)
	}

	switch to {
	case StageBacktested:
		s, err := src.BestBacktestSharpe(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: backtest sharpe lookup: %v", ErrGuardFailed, err)
		}
		if math.IsNaN(s) || s <= 0 {
			return fmt.Errorf("%w: no passing backtest for %s", ErrGuardFailed, strategyID)
		}
		if s < pol.MinBacktestSharpe {
			return fmt.Errorf("%w: best backtest Sharpe %.2f below threshold %.2f",
				ErrGuardFailed, s, pol.MinBacktestSharpe)
		}
		return nil

	case StagePaper:
		// paper from backtested is a manual gate — no quantitative guard.
		return nil

	case StageCanary:
		dur, err := src.ShadowRunDuration(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: shadow duration lookup: %v", ErrGuardFailed, err)
		}
		if dur < pol.MinShadowDuration {
			return fmt.Errorf("%w: shadow ran %s, need %s",
				ErrGuardFailed, dur.Round(time.Second), pol.MinShadowDuration)
		}
		pnl, err := src.ShadowVirtualPnL(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: shadow pnl lookup: %v", ErrGuardFailed, err)
		}
		if pnl < pol.MinShadowVirtualPnL {
			return fmt.Errorf("%w: shadow virtual PnL %.2f below threshold %.2f",
				ErrGuardFailed, pnl, pol.MinShadowVirtualPnL)
		}
		return nil

	case StageLive:
		dur, err := src.CanaryRunDuration(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: canary duration lookup: %v", ErrGuardFailed, err)
		}
		if dur < pol.MinCanaryDuration {
			return fmt.Errorf("%w: canary ran %s, need %s",
				ErrGuardFailed, dur.Round(time.Second), pol.MinCanaryDuration)
		}
		liveSharpe, err := src.CanaryLiveSharpe(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: canary sharpe lookup: %v", ErrGuardFailed, err)
		}
		backtestSharpe, err := src.BestBacktestSharpe(ctx, strategyID)
		if err != nil {
			return fmt.Errorf("%w: backtest sharpe lookup: %v", ErrGuardFailed, err)
		}
		drift := backtestSharpe - liveSharpe
		if drift > pol.MaxSharpeDrift {
			return fmt.Errorf("%w: Sharpe drift %.2f (live %.2f vs backtest %.2f) exceeds %.2f",
				ErrGuardFailed, drift, liveSharpe, backtestSharpe, pol.MaxSharpeDrift)
		}
		return nil
	}
	return fmt.Errorf("%w: no guard wired for target %s", ErrGuardFailed, to)
}

// Transition combines Classify + Check into one call. It is the entry
// point used by the REST handler; the CLI / optimiser can also call it.
// Reason and Actor are stored verbatim in the audit log by the caller;
// this pure function only returns the accepted TransitionKind or an
// error.
func Transition(ctx context.Context, strategyID string, from, to Stage, src EvidenceSource, pol Policy) (TransitionKind, error) {
	kind, err := Classify(from, to)
	if err != nil {
		return "", err
	}
	if errors.Is(Check(ctx, kind, strategyID, from, to, src, pol), ErrGuardFailed) {
		return "", Check(ctx, kind, strategyID, from, to, src, pol)
	}
	return kind, nil
}
