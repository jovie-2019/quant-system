package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAllStages_Ordering(t *testing.T) {
	stages := AllStages()
	if len(stages) != 6 {
		t.Fatalf("len=%d want 6", len(stages))
	}
	for i, want := range []Stage{StageDraft, StageBacktested, StagePaper, StageCanary, StageLive, StageDeprecated} {
		if stages[i] != want {
			t.Fatalf("stage[%d]=%s want %s", i, stages[i], want)
		}
	}
}

func TestStage_Index(t *testing.T) {
	if StageDraft.Index() != 0 || StageLive.Index() != 4 {
		t.Fatalf("indices wrong")
	}
	if Stage("unknown").Index() != -1 {
		t.Fatalf("unknown stage should return -1")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		from, to Stage
		want     TransitionKind
		wantErr  error
	}{
		{StageDraft, StageBacktested, TransitionPromote, nil},
		{StageBacktested, StagePaper, TransitionPromote, nil},
		{StagePaper, StageCanary, TransitionPromote, nil},
		{StageCanary, StageLive, TransitionPromote, nil},
		{StageLive, StageCanary, TransitionDemote, nil},
		{StagePaper, StageBacktested, TransitionDemote, nil},
		{StageLive, StageDeprecated, TransitionDeprecate, nil},
		{StageDraft, StageDeprecated, TransitionDeprecate, nil},

		// Illegal: skipping stages.
		{StageDraft, StageLive, "", ErrIllegalTransition},
		{StageBacktested, StageCanary, "", ErrIllegalTransition},

		// Unknown.
		{Stage("weird"), StageLive, "", ErrUnknownStage},

		// From deprecated: rejected.
		{StageDeprecated, StageLive, "", ErrTerminal},
		{StageDeprecated, StageDraft, "", ErrTerminal},

		// No-op.
		{StageLive, StageLive, "", ErrNoChange},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			got, err := Classify(tc.from, tc.to)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err=%v", err)
			}
			if got != tc.want {
				t.Fatalf("kind=%s want %s", got, tc.want)
			}
		})
	}
}

// fakeEvidence lets tests inject synthetic readings into each guard.
type fakeEvidence struct {
	bestSharpe        float64
	shadowDur         time.Duration
	shadowPnL         float64
	canaryDur         time.Duration
	canarySharpe      float64
	errBestSharpe     error
	errShadowDur      error
	errShadowPnL      error
	errCanaryDur      error
	errCanarySharpe   error
}

func (f *fakeEvidence) BestBacktestSharpe(_ context.Context, _ string) (float64, error) {
	return f.bestSharpe, f.errBestSharpe
}
func (f *fakeEvidence) ShadowRunDuration(_ context.Context, _ string) (time.Duration, error) {
	return f.shadowDur, f.errShadowDur
}
func (f *fakeEvidence) ShadowVirtualPnL(_ context.Context, _ string) (float64, error) {
	return f.shadowPnL, f.errShadowPnL
}
func (f *fakeEvidence) CanaryRunDuration(_ context.Context, _ string) (time.Duration, error) {
	return f.canaryDur, f.errCanaryDur
}
func (f *fakeEvidence) CanaryLiveSharpe(_ context.Context, _ string) (float64, error) {
	return f.canarySharpe, f.errCanarySharpe
}

func TestCheck_PromoteToBacktested_NeedsPassingSharpe(t *testing.T) {
	pol := DefaultPolicy()
	ctx := context.Background()
	src := &fakeEvidence{bestSharpe: 0.5} // below 0.8 threshold
	err := Check(ctx, TransitionPromote, "s1", StageDraft, StageBacktested, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed", err)
	}

	src.bestSharpe = 1.5
	if err := Check(ctx, TransitionPromote, "s1", StageDraft, StageBacktested, src, pol); err != nil {
		t.Fatalf("passing sharpe should succeed: %v", err)
	}
}

func TestCheck_PromoteToPaper_AlwaysPasses(t *testing.T) {
	src := &fakeEvidence{}
	err := Check(context.Background(), TransitionPromote, "s1", StageBacktested, StagePaper, src, DefaultPolicy())
	if err != nil {
		t.Fatalf("paper promotion should be unguarded: %v", err)
	}
}

func TestCheck_PromoteToCanary_NeedsShadowRun(t *testing.T) {
	pol := DefaultPolicy()
	ctx := context.Background()

	// Shadow too short.
	src := &fakeEvidence{shadowDur: time.Hour, shadowPnL: 100}
	err := Check(ctx, TransitionPromote, "s1", StagePaper, StageCanary, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed for short shadow", err)
	}

	// Shadow long enough but negative PnL.
	src = &fakeEvidence{shadowDur: 48 * time.Hour, shadowPnL: -1}
	err = Check(ctx, TransitionPromote, "s1", StagePaper, StageCanary, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed for negative PnL", err)
	}

	// Happy path.
	src = &fakeEvidence{shadowDur: 48 * time.Hour, shadowPnL: 10}
	if err := Check(ctx, TransitionPromote, "s1", StagePaper, StageCanary, src, pol); err != nil {
		t.Fatalf("shadow-clean promotion should succeed: %v", err)
	}
}

func TestCheck_PromoteToLive_NeedsCanaryDurationAndDriftBudget(t *testing.T) {
	pol := DefaultPolicy()
	ctx := context.Background()

	// Canary too short.
	src := &fakeEvidence{canaryDur: time.Hour, canarySharpe: 1.5, bestSharpe: 1.6}
	err := Check(ctx, TransitionPromote, "s1", StageCanary, StageLive, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed for short canary", err)
	}

	// Drift too large: backtest 2.0, live 0.2 → drift 1.8 > 0.6.
	src = &fakeEvidence{canaryDur: 96 * time.Hour, canarySharpe: 0.2, bestSharpe: 2.0}
	err = Check(ctx, TransitionPromote, "s1", StageCanary, StageLive, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed for drift", err)
	}

	// Happy path: Canary matches backtest within budget.
	src = &fakeEvidence{canaryDur: 96 * time.Hour, canarySharpe: 1.6, bestSharpe: 1.8}
	if err := Check(ctx, TransitionPromote, "s1", StageCanary, StageLive, src, pol); err != nil {
		t.Fatalf("canary clean promotion should succeed: %v", err)
	}
}

func TestCheck_DemoteAndDeprecateSkipGuards(t *testing.T) {
	pol := DefaultPolicy()
	ctx := context.Background()
	src := &fakeEvidence{} // no data at all

	// Demotion is unconditional.
	if err := Check(ctx, TransitionDemote, "s1", StageLive, StageCanary, src, pol); err != nil {
		t.Fatalf("demote should skip guard: %v", err)
	}
	// Deprecation is unconditional.
	if err := Check(ctx, TransitionDeprecate, "s1", StageLive, StageDeprecated, src, pol); err != nil {
		t.Fatalf("deprecate should skip guard: %v", err)
	}
}

func TestTransition_Wrapper(t *testing.T) {
	pol := DefaultPolicy()
	ctx := context.Background()

	// Illegal: skipping stages is rejected before Check is called.
	_, err := Transition(ctx, "s1", StageDraft, StageLive, &fakeEvidence{}, pol)
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("err=%v want ErrIllegalTransition", err)
	}

	// Guard failure propagates through.
	src := &fakeEvidence{bestSharpe: 0.1}
	_, err = Transition(ctx, "s1", StageDraft, StageBacktested, src, pol)
	if !errors.Is(err, ErrGuardFailed) {
		t.Fatalf("err=%v want ErrGuardFailed", err)
	}

	// Full happy path.
	src.bestSharpe = 1.5
	kind, err := Transition(ctx, "s1", StageDraft, StageBacktested, src, pol)
	if err != nil {
		t.Fatalf("unexpected err=%v", err)
	}
	if kind != TransitionPromote {
		t.Fatalf("kind=%s want promote", kind)
	}
}
