package contracts

import "encoding/json"

// StrategyControlType discriminates the payload of a StrategyControlEnvelope.
// The small, closed enum keeps the runner's switch statement exhaustive at
// the compiler level and prevents wire-format drift.
type StrategyControlType string

const (
	// StrategyControlUpdateParams replaces the strategy's parameters with
	// the Params field of the envelope. The runner validates + atomically
	// swaps without restarting the process. Internal state (ring buffers,
	// open positions) is preserved when the strategy declares a field is
	// safe to hot-swap; otherwise the runner rejects the update.
	StrategyControlUpdateParams StrategyControlType = "update_params"

	// StrategyControlPause stops the runner from emitting intents but
	// keeps the strategy receiving market events so indicators stay warm.
	StrategyControlPause StrategyControlType = "pause"

	// StrategyControlResume reverses a prior pause.
	StrategyControlResume StrategyControlType = "resume"

	// StrategyControlShadowOn diverts emitted intents to
	// SubjectStrategyShadowIntent instead of the live intent stream so the
	// strategy can be evaluated without any real orders being placed.
	StrategyControlShadowOn StrategyControlType = "shadow_on"

	// StrategyControlShadowOff restores live intent routing.
	StrategyControlShadowOff StrategyControlType = "shadow_off"
)

// StrategyControlEnvelope is the wire format for a control command. The
// Revision field is monotonic per StrategyID and is the audit log's
// primary key — duplicates are rejected.
type StrategyControlEnvelope struct {
	StrategyID string              `json:"strategy_id"`
	Type       StrategyControlType `json:"type"`
	Revision   int64               `json:"revision"`
	Params     json.RawMessage     `json:"params,omitempty"`
	Reason     string              `json:"reason,omitempty"`
	Actor      string              `json:"actor,omitempty"`
	IssuedMS   int64               `json:"issued_ms"`
}

// StrategyControlAck is the runner's reply on SubjectStrategyControlAck.
// admin-api subscribes and updates the audit log row identified by
// (StrategyID, Revision).
type StrategyControlAck struct {
	StrategyID string `json:"strategy_id"`
	Revision   int64  `json:"revision"`
	Accepted   bool   `json:"accepted"`
	Error      string `json:"error,omitempty"`
	AppliedMS  int64  `json:"applied_ms"`
}
