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

// ConfigField describes a single configuration field for a strategy type.
type ConfigField struct {
	Field       string `json:"field"`
	Type        string `json:"type"`        // "string","number","boolean"
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// StrategyMeta holds metadata and config schema for a strategy type.
type StrategyMeta struct {
	Type         string        `json:"type"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	ConfigFields []ConfigField `json:"config_fields"`
}

var metaRegistry = map[string]StrategyMeta{}

// RegisterMeta registers metadata for a strategy type.
func RegisterMeta(meta StrategyMeta) {
	registryMu.Lock()
	defer registryMu.Unlock()

	metaRegistry[meta.Type] = meta
}

// ListMetas returns all registered strategy metadata sorted by type name.
func ListMetas() []StrategyMeta {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(metaRegistry))
	for name := range metaRegistry {
		names = append(names, name)
	}
	sort.Strings(names)

	metas := make([]StrategyMeta, len(names))
	for i, name := range names {
		metas[i] = metaRegistry[name]
	}
	return metas
}

// GetMeta returns the metadata for a strategy type.
func GetMeta(typeName string) (StrategyMeta, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	meta, ok := metaRegistry[typeName]
	return meta, ok
}
