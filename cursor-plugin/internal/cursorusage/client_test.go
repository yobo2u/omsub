package cursorusage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Client_Fetch_combines_dashboard_usage_and_percentages(t *testing.T) {
	var gotPeriod, gotEvents bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "https://cursor.com", request.Header.Get("origin"))
		require.Equal(t, "https://cursor.com/dashboard", request.Header.Get("referer"))
		cookie, err := url.QueryUnescape(request.Header.Get("cookie"))
		require.NoError(t, err)
		require.Equal(t, "WorkosCursorSessionToken=auth0|user-1::access-token", cookie)
		switch request.URL.Path {
		case "/api/usage-summary":
			require.Equal(t, http.MethodGet, request.Method)
			_, _ = writer.Write([]byte(`{
				"billingCycleStart":"2026-08-24T07:46:02.000Z",
				"billingCycleEnd":"2026-09-24T07:46:02.000Z",
				"membershipType":"pro",
				"autoModelSelectedDisplayMessage":"auto used",
				"namedModelSelectedDisplayMessage":"api used",
				"individualUsage":{
					"plan":{
						"limit":2000,
						"autoPercentUsed":49.05111111111111,
						"apiPercentUsed":100,
						"totalPercentUsed":100,
						"breakdown":{"included":2000,"bonus":230908,"total":232908}
					},
					"onDemand":{"used":2338}
				}
			}`))
		case "/api/dashboard/get-current-period-usage":
			require.Equal(t, http.MethodPost, request.Method)
			gotPeriod = true
			_, _ = writer.Write([]byte(`{
				"billingCycleStart":"1787557562000",
				"billingCycleEnd":"1790235962000",
				"displayMessage":"You've hit your usage limit",
				"planUsage":{"totalSpend":232908,"includedSpend":2000,"bonusSpend":230908,"limit":2000,"autoPercentUsed":49.05111111111111,"apiPercentUsed":100,"totalPercentUsed":100},
				"spendLimitUsage":{"totalSpend":2338,"individualUsed":2338,"limitType":"user"}
			}`))
		case "/api/dashboard/get-aggregated-usage-events":
			require.Equal(t, http.MethodPost, request.Method)
			raw, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			var payload aggregatedUsageRequest
			require.NoError(t, json.Unmarshal(raw, &payload))
			require.Equal(t, -1, payload.TeamID)
			require.Equal(t, time.Date(2026, 8, 24, 7, 46, 2, 0, time.UTC).UnixMilli(), payload.StartDate)
			gotEvents = true
			_, _ = writer.Write([]byte(`{
				"aggregations":[
					{"modelIntent":"composer-2","totalCents":22073.44827,"tier":2},
					{"modelIntent":"claude-fable-5-thinking-xhigh","totalCents":210834.65298,"tier":1}
				]
			}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	snapshot, err := client.Fetch(context.Background(), "access-token", "auth0|user-1")

	require.NoError(t, err)
	require.True(t, gotPeriod)
	require.True(t, gotEvents)
	require.Equal(t, "pro", snapshot.MembershipType)
	require.Equal(t, "You've hit your usage limit", snapshot.DisplayMessage)
	require.InDelta(t, 49.05111111111111, snapshot.AutoPercentUsed, 1e-9)
	require.EqualValues(t, 100, snapshot.APIPercentUsed)
	require.NotNil(t, snapshot.CursorModels.UsedUSD)
	require.InDelta(t, 220.7344827, *snapshot.CursorModels.UsedUSD, 1e-6)
	require.NotNil(t, snapshot.CursorModels.EstimatedTotalUSD)
	require.InDelta(t, 220.7344827*100/49.05111111111111, *snapshot.CursorModels.EstimatedTotalUSD, 1e-6)
	require.NotNil(t, snapshot.OtherModels.UsedUSD)
	require.InDelta(t, 2108.3465298, *snapshot.OtherModels.UsedUSD, 1e-6)
	require.NotNil(t, snapshot.OtherModels.GuaranteedUSD)
	require.EqualValues(t, 20, *snapshot.OtherModels.GuaranteedUSD)
	require.InDelta(t, 23.38, snapshot.OnDemandSpendUSD, 1e-9)
}

func Test_Client_Fetch_skips_estimated_total_when_auto_percent_is_complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/usage-summary":
			_, _ = writer.Write([]byte(`{
				"billingCycleStart":"2026-08-24T07:46:02.000Z",
				"individualUsage":{"plan":{"limit":2000,"autoPercentUsed":100,"apiPercentUsed":10}}
			}`))
		case "/api/dashboard/get-current-period-usage":
			_, _ = writer.Write([]byte(`{}`))
		case "/api/dashboard/get-aggregated-usage-events":
			_, _ = writer.Write([]byte(`{"aggregations":[{"modelIntent":"default","totalCents":1000,"tier":2}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	snapshot, err := client.Fetch(context.Background(), "token", "account")

	require.NoError(t, err)
	require.NotNil(t, snapshot.CursorModels.UsedUSD)
	require.Nil(t, snapshot.CursorModels.EstimatedTotalUSD)
}

func Test_Client_Fetch_requires_origin_and_reports_http_errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"error":"Invalid origin for state-changing request"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = client.Fetch(context.Background(), "token", "account")

	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 403")
}

func Test_Client_Fetch_live_dashboard_usage(t *testing.T) {
	token := os.Getenv("CURSOR_TEST_ACCESS_TOKEN")
	accountID := os.Getenv("CURSOR_TEST_ACCOUNT_ID")
	if token == "" || accountID == "" {
		t.Skip("set CURSOR_TEST_ACCESS_TOKEN and CURSOR_TEST_ACCOUNT_ID")
	}
	client, err := NewClient(Config{})
	require.NoError(t, err)

	snapshot, err := client.Fetch(context.Background(), token, accountID)

	require.NoError(t, err)
	require.False(t, snapshot.BillingCycleStart.IsZero())
	require.GreaterOrEqual(t, snapshot.AutoPercentUsed, 0.0)
	require.GreaterOrEqual(t, snapshot.APIPercentUsed, 0.0)
}
