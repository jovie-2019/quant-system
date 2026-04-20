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

// PublishStrategyControl sends a lifecycle command to the runner that
// owns the given strategy. The admin-api is the typical publisher;
// scheduled optimisation pipelines can also use this path when an auto-
// promotion has been approved.
func PublishStrategyControl(ctx context.Context, client *Client, env contracts.StrategyControlEnvelope, header map[string]string) error {
	if env.StrategyID == "" {
		return fmt.Errorf("natsbus: strategy_id is empty")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return client.Publish(ctx, SubjectStrategyControl(env.StrategyID), data, header)
}

// PublishStrategyControlAck reports the outcome of a control command back
// to any interested subscriber (admin-api's audit-log listener in the
// typical deployment).
func PublishStrategyControlAck(ctx context.Context, client *Client, ack contracts.StrategyControlAck, header map[string]string) error {
	if ack.StrategyID == "" {
		return fmt.Errorf("natsbus: strategy_id is empty")
	}
	data, err := json.Marshal(ack)
	if err != nil {
		return err
	}
	return client.Publish(ctx, SubjectStrategyControlAck(ack.StrategyID), data, header)
}

// PublishShadowIntent publishes an order intent to the shadow subject when
// the strategy is operating in shadow mode. Intended for low-cardinality
// routing; the payload format mirrors PublishStrategyIntent.
func PublishShadowIntent(ctx context.Context, client *Client, intent contracts.OrderIntent, header map[string]string) error {
	if intent.StrategyID == "" {
		return fmt.Errorf("natsbus: strategy_id is empty")
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return client.Publish(ctx, SubjectStrategyShadowIntent(intent.StrategyID), data, header)
}
