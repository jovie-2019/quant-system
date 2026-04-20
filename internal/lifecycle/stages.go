// Package lifecycle models the promotion pipeline a strategy moves
// through from first draft to full production and, eventually, retirement.
//
// Six stages:
//
//	draft       — config created, never backtested
//	backtested  — has at least one completed backtest that passed
//	              gates (finite Sharpe, positive trade count)
//	paper       — running in shadow mode; producing signals but not
//	              placing real orders
//	canary      — live but with a capped position size / budget; used
//	              to validate out-of-sample behaviour before full rollout
//	live        — full production
//	deprecated  — retired; no further transitions
//
// The state machine is intentionally linear with one escape hatch: any
// stage can transition to deprecated (operator kill switch) and any
// non-terminal stage can transition backwards to the immediately prior
// stage (automatic health-drift demotion). Forward promotion requires a
// Guard to pass — the Guards are provided by the caller so the pure
// transition logic stays testable without stubbing databases.
package lifecycle

import (
	"errors"
	"fmt"
)

// Stage enumerates the six lifecycle positions. Values are strings so
// they serialise naturally into JSON and MySQL without an enum column.
type Stage string

const (
	StageDraft      Stage = "draft"
	StageBacktested Stage = "backtested"
	StagePaper      Stage = "paper"
	StageCanary     Stage = "canary"
	StageLive       Stage = "live"
	StageDeprecated Stage = "deprecated"
)

// AllStages returns the canonical ordered list used by UIs rendering a
// Kanban-style board. The order matches the forward promotion flow.
func AllStages() []Stage {
	return []Stage{
		StageDraft,
		StageBacktested,
		StagePaper,
		StageCanary,
		StageLive,
		StageDeprecated,
	}
}

// Valid reports whether s is a known stage.
func (s Stage) Valid() bool {
	switch s {
	case StageDraft, StageBacktested, StagePaper, StageCanary, StageLive, StageDeprecated:
		return true
	}
	return false
}

// Index returns the zero-based position of the stage in the forward
// promotion flow; -1 for unknown stages. Useful for ordinal comparisons
// like "is this stage more advanced than live".
func (s Stage) Index() int {
	for i, v := range AllStages() {
		if v == s {
			return i
		}
	}
	return -1
}

// Common errors returned by Transition.
var (
	ErrUnknownStage       = errors.New("lifecycle: unknown stage")
	ErrTerminal           = errors.New("lifecycle: deprecated is terminal")
	ErrIllegalTransition  = errors.New("lifecycle: illegal transition")
	ErrGuardFailed        = errors.New("lifecycle: guard check failed")
	ErrNoChange           = errors.New("lifecycle: from == to")
)

// TransitionKind categorises how a transition is being invoked.
// Demotions and kill-switches skip forward-promotion guards.
type TransitionKind string

const (
	TransitionPromote    TransitionKind = "promote"
	TransitionDemote     TransitionKind = "demote"
	TransitionDeprecate  TransitionKind = "deprecate"
)

// Classify returns the kind of transition implied by (from, to).
func Classify(from, to Stage) (TransitionKind, error) {
	if !from.Valid() || !to.Valid() {
		return "", ErrUnknownStage
	}
	if from == to {
		return "", ErrNoChange
	}
	if from == StageDeprecated {
		return "", ErrTerminal
	}
	if to == StageDeprecated {
		return TransitionDeprecate, nil
	}
	fi, ti := from.Index(), to.Index()
	if ti == fi+1 {
		return TransitionPromote, nil
	}
	if ti == fi-1 {
		return TransitionDemote, nil
	}
	return "", fmt.Errorf("%w: %s -> %s (only adjacent moves allowed)", ErrIllegalTransition, from, to)
}
