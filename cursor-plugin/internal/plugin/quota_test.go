package plugin

import (
	"context"
	"cursorplugin/internal/cursorapi"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type quotaTestClient struct {
	fakeModelCursorClient
	usage cursorapi.PeriodUsage
	err   error
	calls *int
}

func (c quotaTestClient) CurrentPeriodUsage(context.Context, string) (cursorapi.PeriodUsage, error) {
	*c.calls++
	return c.usage, c.err
}
func (c quotaTestClient) CurrentPlanInfo(context.Context, string) (cursorapi.PlanInfo, error) {
	return cursorapi.PlanInfo{PlanName: "Pro"}, nil
}

func TestQuotaFetchAndCache(t *testing.T) {
	calls := 0
	auto, api := 2.0, 50.0
	client := quotaTestClient{calls: &calls, usage: cursorapi.PeriodUsage{BillingCycleEnd: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Plan: &cursorapi.PlanUsage{AutoPercentUsed: &auto, APIPercentUsed: &api, TotalSpend: 967}}}
	h := NewHandler(Dependencies{Cursor: client})
	request, _ := json.Marshal(map[string]any{"storage_json": []byte(`{"type":"cursor","access_token":"synthetic-token","refresh_token":"synthetic-refresh"}`)})
	for range 3 {
		raw, ok := h.CallWithStatus(context.Background(), "quota.fetch", request)
		require.True(t, ok, string(raw))
		var envelope struct{ Result quotaResponse }
		require.NoError(t, json.Unmarshal(raw, &envelope))
		require.Equal(t, "Pro", envelope.Result.Subscription.Plan)
		require.Empty(t, envelope.Result.Summary)
		require.Len(t, envelope.Result.Groups, 1)
		buckets := envelope.Result.Groups[0].Buckets
		require.Len(t, buckets, 2)
		require.Equal(t, "Cursor Models (Grok + Composer)", buckets[0].Description)
		require.InDelta(t, 0.98, buckets[0].RemainingFraction, 0.0001)
		require.Equal(t, "Other Models", buckets[1].Description)
		require.Equal(t, 0.5, buckets[1].RemainingFraction)
		require.Equal(t, "2027-01-01T00:00:00Z", buckets[0].ResetTime)
		require.NotContains(t, string(raw), "synthetic-token")
	}
	require.Equal(t, 1, calls)
}
func TestQuotaErrorsCached(t *testing.T) {
	calls := 0
	h := NewHandler(Dependencies{Cursor: quotaTestClient{calls: &calls, err: &cursorapi.DashboardError{Method: "GetCurrentPeriodUsage", StatusCode: 429}}})
	for range 3 {
		result := h.quotaFor(context.Background(), "synthetic-token")
		require.Contains(t, result.Err, "HTTP 429")
		require.NotContains(t, result.Err, "synthetic-token")
	}
	require.Equal(t, 1, calls)
}
func TestQuotaOnDemand(t *testing.T) {
	result := buildQuotaResponse(cursorapi.PeriodUsage{SpendLimit: &cursorapi.SpendLimitUsage{IndividualLimit: 5000, IndividualUsed: 1250}}, cursorapi.PlanInfo{})
	require.Len(t, result.Groups, 1)
	require.Equal(t, 0.75, result.Groups[0].Buckets[0].RemainingFraction)
	require.Equal(t, 12.5, result.Summary[0].Value)
	empty := buildQuotaResponse(cursorapi.PeriodUsage{SpendLimit: &cursorapi.SpendLimitUsage{}}, cursorapi.PlanInfo{})
	require.Empty(t, empty.Groups)
	require.Empty(t, empty.Summary)
}
func TestQuotaCacheExpiry(t *testing.T) {
	var cache ttlCache[int]
	now := time.Now()
	cache.put("a", 1, time.Minute, now)
	_, ok := cache.get("a", now)
	require.True(t, ok)
	_, ok = cache.get("a", now.Add(time.Minute))
	require.False(t, ok)
}
