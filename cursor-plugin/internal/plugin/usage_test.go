package plugin

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageStore_BoundsAbandonedRequestAuthUnderConcurrentTraffic(t *testing.T) {
	store := newUsageStore()
	const requests = 5000

	var workers sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for index := offset; index < requests; index += 32 {
				store.selectRequestAuth(fmt.Sprintf("request-%d", index), "cursor-account.json")
			}
		}(worker)
	}
	workers.Wait()

	store.mu.RLock()
	pending := len(store.requestAuth)
	ordered := store.requestAuthOrder.Len()
	store.mu.RUnlock()
	require.Equal(t, requestAuthCapacity, pending)
	require.Equal(t, pending, ordered)
}

func TestUsageStore_ExpiresAbandonedRequestAuth(t *testing.T) {
	store := newUsageStore()
	now := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	store.selectRequestAuth("abandoned", "cursor-old.json")
	now = now.Add(requestAuthTTL + time.Second)
	store.selectRequestAuth("current", "cursor-current.json")
	store.completeRequest("abandoned", "", false, "succeeded")
	store.completeRequest("current", "", false, "succeeded")

	store.mu.RLock()
	pending := len(store.requestAuth)
	ordered := store.requestAuthOrder.Len()
	store.mu.RUnlock()
	require.Zero(t, pending)
	require.Zero(t, ordered)
	require.Zero(t, store.snapshot("cursor-old.json").Requests)
	require.EqualValues(t, 1, store.snapshot("cursor-current.json").Requests)
}
