package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// ttlCache stores short-lived responses without keeping raw account tokens as keys.
type ttlCache[T any] struct {
	mu      sync.Mutex
	entries map[string]ttlEntry[T]
}
type ttlEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func (cache *ttlCache[T]) get(key string, now time.Time) (T, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok || !now.Before(entry.expiresAt) {
		delete(cache.entries, key)
		var zero T
		return zero, false
	}
	return entry.value, true
}
func (cache *ttlCache[T]) put(key string, value T, ttl time.Duration, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = make(map[string]ttlEntry[T])
	}
	for existing, entry := range cache.entries {
		if !now.Before(entry.expiresAt) {
			delete(cache.entries, existing)
		}
	}
	cache.entries[key] = ttlEntry[T]{value: value, expiresAt: now.Add(ttl)}
}
func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
