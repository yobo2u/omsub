package cursorapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Client_CurrentPeriodUsage_reads_dashboard_period_plan_and_limit(t *testing.T) {
	var methods []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.URL.Path)
		require.Equal(t, "Bearer access-token", request.Header.Get("authorization"))
		require.Equal(t, "application/json", request.Header.Get("content-type"))
		require.Equal(t, "1", request.Header.Get("connect-protocol-version"))
		switch request.URL.Path {
		case "/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
			_, _ = response.Write([]byte(`{
				"billingCycleEnd":"1712592000000",
				"planUsage":{"includedSpend":800,"limit":2000,"totalPercentUsed":40,"autoPercentUsed":"10","apiPercentUsed":30},
				"spendLimitUsage":{"individualUsed":250,"individualLimit":"5000","limitType":"user"}
			}`))
		case "/aiserver.v1.DashboardService/GetPlanInfo":
			_, _ = response.Write([]byte(`{"planInfo":{"planName":"Pro","billingCycleEnd":"1712592000000"}}`))
		case "/aiserver.v1.DashboardService/GetHardLimit":
			_, _ = response.Write([]byte(`{"hardLimit":20,"noUsageBasedAllowed":false}`))
		case "/aiserver.v1.DashboardService/GetSandUsageStatus":
			_, _ = response.Write([]byte(`{"currentPeriodStart":"2026-09-15T20:23:06.799Z","nextResetTimestampUtc":"2026-09-22T20:23:06.799Z","usagePercent":6.93,"grokPlanLabel":"Grok Bot Plan"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	usage, err := client.CurrentPeriodUsage(context.Background(), "access-token")

	require.NoError(t, err)
	require.Equal(t, []string{
		"/aiserver.v1.DashboardService/GetCurrentPeriodUsage",
		"/aiserver.v1.DashboardService/GetPlanInfo",
		"/aiserver.v1.DashboardService/GetHardLimit",
		"/aiserver.v1.DashboardService/GetSandUsageStatus",
	}, methods)
	require.Equal(t, "Pro", usage.PlanName)
	require.Equal(t, 40.0, usage.IncludedPercentUsed)
	require.Equal(t, 10.0, usage.AutoPercentUsed)
	require.Equal(t, 30.0, usage.APIPercentUsed)
	require.True(t, usage.HasIncludedSpend)
	require.EqualValues(t, 800, usage.IncludedSpendCents)
	require.EqualValues(t, 2000, usage.IncludedLimitCents)
	require.Equal(t, time.UnixMilli(1712592000000).UTC(), usage.BillingCycleEnd)
	require.Equal(t, "fixed", usage.OnDemandKind)
	require.EqualValues(t, 250, usage.OnDemandUsedCents)
	require.EqualValues(t, 2000, usage.OnDemandLimitCents)
	require.True(t, usage.HasGrokBotPercent)
	require.Equal(t, 6.93, usage.GrokBotPercentUsed)
	require.Equal(t, "Grok Bot Plan", usage.GrokBotLabel)
	require.Equal(t, time.Date(2026, 9, 22, 20, 23, 6, 799000000, time.UTC), usage.GrokBotResetsAt)
}

func Test_Client_CurrentPeriodUsage_rejects_non_success_dashboard_status(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"message":"nope"}`))
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = client.CurrentPeriodUsage(context.Background(), "access-token")

	require.ErrorContains(t, err, "GetCurrentPeriodUsage returned HTTP 401")
	require.NotContains(t, err.Error(), "access-token")
}
