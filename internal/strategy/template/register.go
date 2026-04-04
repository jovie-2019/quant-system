package template

// Uncomment the code below to register your strategy type.
// Replace "template" with your strategy's type name.
//
// import (
// 	"encoding/json"
// 	"fmt"
//
// 	"quant-system/internal/strategy"
// )
//
// func init() {
// 	strategy.RegisterType("template", func(configJSON json.RawMessage) (strategy.Strategy, error) {
// 		var cfg struct {
// 			Symbol string `json:"symbol"`
// 			// add your config fields here
// 		}
// 		if err := json.Unmarshal(configJSON, &cfg); err != nil {
// 			return nil, fmt.Errorf("template: invalid config: %w", err)
// 		}
// 		if cfg.Symbol == "" {
// 			return nil, fmt.Errorf("template: symbol is required")
// 		}
// 		return New(Config{
// 			Symbol: cfg.Symbol,
// 		}), nil
// 	})
// }
