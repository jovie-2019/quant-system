package template

import (
	"quant-system/internal/strategy"
)

func init() {
	strategy.RegisterMeta(strategy.StrategyMeta{
		Type:        "template",
		Name:        "策略模板",
		Description: "这是一个策略开发模板，不产生任何交易信号。复制此策略目录作为开发新策略的起点。",
		ConfigFields: []strategy.ConfigField{
			{Field: "symbol", Type: "string", Required: true, Default: "", Description: "交易对"},
		},
	})
}
