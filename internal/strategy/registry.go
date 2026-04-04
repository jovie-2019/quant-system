package strategy

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// StrategyConstructor creates a Strategy from JSON config.
type StrategyConstructor func(configJSON json.RawMessage) (Strategy, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]StrategyConstructor{}
)

// RegisterType adds a strategy type to the global registry.
// It panics if the same typeName is registered twice.
func RegisterType(typeName string, ctor StrategyConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, dup := registry[typeName]; dup {
		panic(fmt.Sprintf("strategy: duplicate type registration: %s", typeName))
	}
	registry[typeName] = ctor
}

// Lookup returns the constructor for a strategy type.
func Lookup(typeName string) (StrategyConstructor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	ctor, ok := registry[typeName]
	return ctor, ok
}

// RegisteredTypes returns all registered type names in sorted order.
func RegisteredTypes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
