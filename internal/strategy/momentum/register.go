package momentum

import (
	"encoding/json"
	"fmt"
	"time"

	"quant-system/internal/strategy"
)

func init() {
	strategy.RegisterMeta(strategy.StrategyMeta{
		Type:        "momentum",
		Name:        "动量突破策略",
		Description: "维护固定大小的滚动价格窗口，实时计算窗口内最高价和最低价。当价格突破窗口高点×(1+阈值)时产生买入信号，当价格跌破窗口低点×(1-阈值)且有持仓时产生卖出信号。内置冷却期防止连续出信号。适合趋势明显的行情。",
		ConfigFields: []strategy.ConfigField{
			{Field: "symbol", Type: "string", Required: true, Default: "", Description: "交易对，如 BTC-USDT"},
			{Field: "window_size", Type: "number", Required: false, Default: "20", Description: "滚动窗口大小（tick数）"},
			{Field: "breakout_threshold", Type: "number", Required: false, Default: "0.001", Description: "突破阈值，0.001 表示 0.1%"},
			{Field: "order_qty", Type: "number", Required: true, Default: "", Description: "每笔下单数量"},
			{Field: "time_in_force", Type: "string", Required: false, Default: "IOC", Description: "订单有效期：IOC 或 GTC"},
			{Field: "cooldown_ms", Type: "number", Required: false, Default: "5000", Description: "信号冷却期（毫秒），0 表示不冷却"},
		},
	})

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
