package cursorapi

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardUsage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "GetPlanInfo") {
			w.Write([]byte(`{"planInfo":{"planName":"Pro","billingCycleEnd":"1800000000000"}}`))
			return
		}
		w.Write([]byte(`{"billingCycleEnd":"1800000000000","planUsage":{"autoPercentUsed":2,"apiPercentUsed":50}}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	usage, err := client.CurrentPeriodUsage(context.Background(), "synthetic-token")
	require.NoError(t, err)
	require.Equal(t, 2.0, *usage.Plan.AutoPercentUsed)
	require.Equal(t, int64(1800000000000), usage.BillingCycleEnd.UnixMilli())
	plan, err := client.CurrentPlanInfo(context.Background(), "synthetic-token")
	require.NoError(t, err)
	require.Equal(t, "Pro", plan.PlanName)
}
func TestDashboardFailures(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate_limit", 429, "private upstream detail"},
		{"malformed", 200, "{"},
		{"oversized", 200, strings.Repeat(" ", maxDashboardResponseBytes+1)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(fixture.status)
				w.Write([]byte(fixture.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
			require.NoError(t, err)
			_, err = client.CurrentPeriodUsage(context.Background(), "synthetic-token")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private upstream detail")
			require.NotContains(t, err.Error(), "synthetic-token")
		})
	}
}
