package prompting

import (
	"testing"
	"time"
)

func TestCacheSetGetClear(t *testing.T) {
	cache := NewCache(time.Minute)
	result := BuildResult{Model: "gpt-4.1-mini", Prompt: "hello"}
	cache.Set("k1", result)
	got, ok := cache.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Model != "gpt-4.1-mini" || !got.CacheHit {
		t.Fatalf("unexpected cache result: %#v", got)
	}
	stats := cache.Stats()
	if stats.Entries != 1 || stats.Hits != 1 {
		t.Fatalf("unexpected cache stats: %#v", stats)
	}
	cache.Clear()
	if _, ok := cache.Get("k1"); ok {
		t.Fatal("expected cache miss after clear")
	}
}
