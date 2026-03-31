package ttlcache

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"quant-system/internal/obs/metrics"
)

func TestGetSetBasic(t *testing.T) {
	c := New[string](time.Hour, 100)
	c.Set("k1", "v1")

	v, ok := c.Get("k1")
	if !ok || v != "v1" {
		t.Fatalf("expected v1, got %v found=%v", v, ok)
	}

	_, ok = c.Get("missing")
	if ok {
		t.Fatal("expected not found for missing key")
	}
}

func TestExpiration(t *testing.T) {
	c := New[int](50*time.Millisecond, 0)
	c.Set("k1", 42)

	v, ok := c.Get("k1")
	if !ok || v != 42 {
		t.Fatalf("expected 42, got %v found=%v", v, ok)
	}

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get("k1")
	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestMaxSize(t *testing.T) {
	c := New[int](time.Hour, 3)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if c.Len() != 3 {
		t.Fatalf("expected len 3, got %d", c.Len())
	}

	c.Set("d", 4)
	if c.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", c.Len())
	}

	// "d" must be present.
	v, ok := c.Get("d")
	if !ok || v != 4 {
		t.Fatalf("expected 4, got %v found=%v", v, ok)
	}
}

func TestMaxSizeOverwriteNoEvict(t *testing.T) {
	c := New[int](time.Hour, 2)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("a", 10) // overwrite existing — should not evict.

	if c.Len() != 2 {
		t.Fatalf("expected len 2, got %d", c.Len())
	}
	v, ok := c.Get("a")
	if !ok || v != 10 {
		t.Fatalf("expected 10, got %v found=%v", v, ok)
	}
}

func TestPurgeViaStart(t *testing.T) {
	c := New[int](50*time.Millisecond, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	c.Set("k1", 1)
	c.Set("k2", 2)

	time.Sleep(150 * time.Millisecond)

	if c.Len() != 0 {
		t.Fatalf("expected purged cache, got len=%d", c.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New[int](time.Hour, 10000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			c.Set(key, n)
			c.Get(key)
		}(i)
	}
	wg.Wait()
}

func TestNamedCacheEmitsObservabilityMetrics(t *testing.T) {
	metrics.ResetForTest()

	c := NewNamed[int]("risk_decision", 10*time.Millisecond, 1)
	c.Set("a", 1)

	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected hit for key a")
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss for missing key")
	}

	// Capacity eviction when inserting a new key at max size.
	c.Set("b", 2)

	// Force an expiration purge path.
	time.Sleep(15 * time.Millisecond)
	c.purgeExpired()

	out := metrics.ExposePrometheus()
	if !strings.Contains(out, `engine_ttlcache_get_total{cache="risk_decision",result="hit"} 1`) {
		t.Fatalf("expected ttlcache hit metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_get_total{cache="risk_decision",result="miss"} 1`) {
		t.Fatalf("expected ttlcache miss metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_eviction_total{cache="risk_decision",reason="capacity"} 1`) {
		t.Fatalf("expected ttlcache capacity eviction metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_purge_total{cache="risk_decision"} 1`) {
		t.Fatalf("expected ttlcache purge metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_size{cache="risk_decision"} 0`) {
		t.Fatalf("expected ttlcache size gauge to reach zero, got: %s", out)
	}
}
