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
