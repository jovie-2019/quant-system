package marketstore

import (
	"context"
	"sort"
	"sync"

	"quant-system/pkg/contracts"
)

// MemoryStore is an in-memory KlineStore used by unit tests and for local
// development without ClickHouse. It holds klines in per-key slices kept
// sorted by OpenTime, so Query is O(log n) bounded.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]contracts.Kline
}

// NewMemoryStore returns an empty, ready-to-use in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string][]contracts.Kline)}
}

// key returns the storage key for a (venue, symbol, interval) triple.
func memKey(venue, symbol, interval string) string {
	return NormaliseVenue(venue) + "|" + NormaliseSymbol(symbol) + "|" + NormaliseInterval(interval)
}

// Upsert inserts or replaces klines in the store. The input slice may be in
// any order; internally, rows are inserted so the per-key slice stays sorted
// by OpenTime ASC.
func (s *MemoryStore) Upsert(_ context.Context, klines []contracts.Kline) error {
	if len(klines) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range klines {
		key := memKey(string(k.Venue), k.Symbol, k.Interval)
		bucket := s.data[key]
		idx := sort.Search(len(bucket), func(i int) bool { return bucket[i].OpenTime >= k.OpenTime })
		k.Symbol = NormaliseSymbol(k.Symbol)
		k.Interval = NormaliseInterval(k.Interval)
		if idx < len(bucket) && bucket[idx].OpenTime == k.OpenTime {
			bucket[idx] = k
		} else {
			bucket = append(bucket, contracts.Kline{})
			copy(bucket[idx+1:], bucket[idx:])
			bucket[idx] = k
		}
		s.data[key] = bucket
	}
	return nil
}

// Query returns klines matching the query, ordered by OpenTime ASC.
func (s *MemoryStore) Query(_ context.Context, q KlineQuery) ([]contracts.Kline, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := matchingKeys(s.data, q)
	out := make([]contracts.Kline, 0, 256)
	for _, key := range keys {
		for _, k := range s.data[key] {
			if q.StartMS > 0 && k.OpenTime < q.StartMS {
				continue
			}
			if q.EndMS > 0 && k.OpenTime > q.EndMS {
				break
			}
			out = append(out, k)
			if q.Limit > 0 && len(out) >= q.Limit {
				return out, nil
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenTime < out[j].OpenTime })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// Count returns the number of klines matching the query, ignoring Limit.
func (s *MemoryStore) Count(_ context.Context, q KlineQuery) (int64, error) {
	if err := q.Validate(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var n int64
	for _, key := range matchingKeys(s.data, q) {
		for _, k := range s.data[key] {
			if q.StartMS > 0 && k.OpenTime < q.StartMS {
				continue
			}
			if q.EndMS > 0 && k.OpenTime > q.EndMS {
				break
			}
			n++
		}
	}
	return n, nil
}

// Ping is a no-op for the in-memory store.
func (s *MemoryStore) Ping(_ context.Context) error { return nil }

// Close is a no-op for the in-memory store.
func (s *MemoryStore) Close() error { return nil }

// matchingKeys returns the map keys whose (venue, symbol, interval) match
// the query. Venue is optional: empty matches any venue.
func matchingKeys(data map[string][]contracts.Kline, q KlineQuery) []string {
	out := make([]string, 0, 1)
	sym := NormaliseSymbol(q.Symbol)
	interval := NormaliseInterval(q.Interval)
	for key := range data {
		venue, symbol, inter := splitKey(key)
		if symbol != sym || inter != interval {
			continue
		}
		if q.Venue != "" && venue != NormaliseVenue(q.Venue) {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func splitKey(key string) (venue, symbol, interval string) {
	// key format: "<venue>|<symbol>|<interval>"
	i := 0
	start := 0
	parts := [3]string{}
	for j := 0; j < len(key) && i < 3; j++ {
		if key[j] == '|' {
			parts[i] = key[start:j]
			i++
			start = j + 1
		}
	}
	if i < 3 {
		parts[i] = key[start:]
	}
	return parts[0], parts[1], parts[2]
}
