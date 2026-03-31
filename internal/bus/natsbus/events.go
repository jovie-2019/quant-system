package natsbus

import (
	"context"
	"encoding/json"
	"fmt"

	"quant-system/pkg/contracts"
)

func PublishMarketNormalized(ctx context.Context, client *Client, evt contracts.MarketNormalizedEvent, header map[string]string) error {
	subject := SubjectMarketNormalizedSpot(string(evt.Venue), evt.Symbol)
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return client.Publish(ctx, subject, data, header)
}

func PublishStrategyIntent(ctx context.Context, client *Client, intent contracts.OrderIntent, header map[string]string) error {
	if client == nil {
		return ErrInvalidConfig
	}
	if intent.StrategyID == "" {
		return fmt.Errorf("natsbus: strategy_id is empty")
	}
	subject := SubjectStrategyIntent(intent.StrategyID)
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return client.Publish(ctx, subject, data, header)
}

func PublishRiskDecision(ctx context.Context, client *Client, accountID string, decision contracts.RiskDecision, header map[string]string) error {
	if accountID == "" {
		return fmt.Errorf("natsbus: accountID is empty")
	}
	subject := SubjectRiskDecision(accountID)
	data, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	return client.Publish(ctx, subject, data, header)
}

func PublishOrderLifecycle(ctx context.Context, client *Client, accountID string, evt contracts.OrderLifecycleEvent, header map[string]string) error {
	if accountID == "" {
		return fmt.Errorf("natsbus: accountID is empty")
	}
	subject := SubjectOrderLifecycle(accountID, evt.Symbol)
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return client.Publish(ctx, subject, data, header)
}

func PublishTradeFill(ctx context.Context, client *Client, accountID string, evt contracts.TradeFillEvent, header map[string]string) error {
	if accountID == "" {
		return fmt.Errorf("natsbus: accountID is empty")
	}
	subject := SubjectTradeFill(accountID, evt.Symbol)
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return client.Publish(ctx, subject, data, header)
}
