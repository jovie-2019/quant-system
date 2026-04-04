// Package template is a fully commented example strategy for developers to copy
// and modify. It does not generate any order intents.
//
// Steps to create a new strategy:
//  1. Copy this directory to internal/strategy/yourstrategy/
//  2. Rename the package
//  3. Implement the Strategy interface (ID, OnMarket)
//  4. Create a register.go with init() that calls strategy.RegisterType
//  5. Import the package in cmd/strategy-runner/main.go (blank import)
//  6. Rebuild the image
//  7. Create strategy config in admin panel, click Start
package template

import (
	"quant-system/pkg/contracts"
)

// Config holds strategy parameters. Add your own fields here.
type Config struct {
	Symbol string
	// Add your strategy parameters here
}

// Strategy implements the strategy.Strategy interface.
type Strategy struct {
	cfg Config
}

// New creates a new template Strategy.
func New(cfg Config) *Strategy {
	return &Strategy{cfg: cfg}
}

// ID returns the unique identifier for this strategy instance.
func (s *Strategy) ID() string {
	return "template-" + s.cfg.Symbol
}

// OnMarket is called for each incoming market event.
func (s *Strategy) OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent {
	// Your strategy logic here:
	// 1. Check if this event is for your symbol
	// 2. Update your internal state (indicators, signals)
	// 3. Decide whether to generate an order intent
	// 4. Return order intents (or nil for no action)

	return nil
}
