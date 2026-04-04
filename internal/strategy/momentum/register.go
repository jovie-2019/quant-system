package momentum

import (
	"encoding/json"
	"fmt"
	"time"

	"quant-system/internal/strategy"
)

func init() {
	strategy.RegisterType("momentum", func(configJSON json.RawMessage) (strategy.Strategy, error) {
		var cfg struct {
			Symbol            string  `json:"symbol"`
			WindowSize        int     `json:"window_size"`
			BreakoutThreshold float64 `json:"breakout_threshold"`
			OrderQty          float64 `json:"order_qty"`
			TimeInForce       string  `json:"time_in_force"`
			CooldownMS        int     `json:"cooldown_ms"`
		}
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("momentum: invalid config: %w", err)
		}
		if cfg.Symbol == "" {
			return nil, fmt.Errorf("momentum: symbol is required")
		}
		return New(Config{
			Symbol:            cfg.Symbol,
			WindowSize:        cfg.WindowSize,
			BreakoutThreshold: cfg.BreakoutThreshold,
			OrderQty:          cfg.OrderQty,
			TimeInForce:       cfg.TimeInForce,
			Cooldown:          time.Duration(cfg.CooldownMS) * time.Millisecond,
		}), nil
	})
}
