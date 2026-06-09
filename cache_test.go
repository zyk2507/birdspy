package main

import (
	"testing"
	"time"
)

func TestCacheEvictsLeastRecentlyUsedEntry(t *testing.T) {
	cache := NewCache(time.Hour, 2, 0)
	t.Cleanup(cache.Close)

	cache.Set("a", "first")
	cache.Set("b", "second")
	if value, ok := cache.Peek("a"); !ok || value != "first" {
		t.Fatalf("expected key a before eviction, got value=%v ok=%v", value, ok)
	}

	cache.Set("c", "third")

	if _, ok := cache.Peek("b"); ok {
		t.Fatal("expected least recently used key b to be evicted")
	}
	if value, ok := cache.Peek("a"); !ok || value != "first" {
		t.Fatalf("expected recently used key a to remain, got value=%v ok=%v", value, ok)
	}
	if value, ok := cache.Peek("c"); !ok || value != "third" {
		t.Fatalf("expected key c to remain, got value=%v ok=%v", value, ok)
	}
}

func TestCacheDeleteExpired(t *testing.T) {
	cache := NewCache(5*time.Millisecond, 10, 0)
	t.Cleanup(cache.Close)

	cache.Set("expired", "value")
	time.Sleep(15 * time.Millisecond)

	cache.DeleteExpired()

	if cache.Len() != 0 {
		t.Fatalf("expected expired cache entry to be deleted, got len=%d", cache.Len())
	}
}

func TestCachePeriodicCleanup(t *testing.T) {
	cache := NewCache(5*time.Millisecond, 10, 5*time.Millisecond)
	t.Cleanup(cache.Close)

	cache.Set("expired", "value")

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cache.Len() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected periodic cleanup to delete expired entry, got len=%d", cache.Len())
}
