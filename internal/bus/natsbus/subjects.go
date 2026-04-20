package natsbus

import "fmt"

func SubjectMarketNormalizedSpot(venue, symbol string) string {
	return fmt.Sprintf("market.normalized.spot.%s.%s", venue, symbol)
}

func SubjectStrategyIntent(strategyID string) string {
	return fmt.Sprintf("strategy.intent.%s", strategyID)
}

func SubjectRiskDecision(accountID string) string {
	return fmt.Sprintf("risk.decision.%s", accountID)
}

func SubjectOrderLifecycle(accountID, symbol string) string {
	return fmt.Sprintf("order.lifecycle.%s.%s", accountID, symbol)
}

func SubjectTradeFill(accountID, symbol string) string {
	return fmt.Sprintf("trade.fill.%s.%s", accountID, symbol)
}

// SubjectStrategyControl is the NATS subject on which admin-api publishes
// lifecycle commands (update_params, pause, resume, shadow_on, shadow_off)
// for a specific strategy. The strategy-runner process subscribes with a
// consumer named "runner-ctl-<strategy_id>" so only the owning runner
// receives the message.
func SubjectStrategyControl(strategyID string) string {
	return fmt.Sprintf("strategy.control.%s", strategyID)
}

// SubjectStrategyControlAck is the subject on which the runner reports the
// outcome of a control command. admin-api subscribes here to record the
// result into the audit log.
func SubjectStrategyControlAck(strategyID string) string {
	return fmt.Sprintf("strategy.control.ack.%s", strategyID)
}

// SubjectStrategyShadowIntent is the subject used when a strategy is in
// shadow mode: the runner emits intents here instead of the live
// strategy.intent.* stream, so a downstream consumer can measure virtual
// PnL without the order ever reaching the exchange.
func SubjectStrategyShadowIntent(strategyID string) string {
	return fmt.Sprintf("strategy.shadow.%s", strategyID)
}
