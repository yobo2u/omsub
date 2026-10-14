package cursorapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ModelContextLengths_omits_missing_invalid_and_max_only_limits(t *testing.T) {
	// Given
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"models":[
			{"name":"missing","contextTokenLimitForMaxMode":1000000},
			{"name":"negative","contextTokenLimit":-1},
			{"name":"max-only","contextTokenLimit":1000000,"supportsNonMaxMode":false},
			{"name":"base","serverModelName":"server","contextTokenLimit":123456,"variants":[{"legacySlug":"max-variant","isMaxMode":true}]}
		]}`))
		require.NoError(t, err)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)

	// When
	limits, err := client.ModelContextLengths(context.Background(), "token")

	// Then
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"base": 123456, "server": 123456}, limits)
}

func Test_EditorCatalog_filters_chat_and_max_metadata(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"family","clientDisplayName":"Family","supportsAgent":true,"contextTokenLimit":200000,"legacySlugs":["family","family"],"variants":[{"legacySlug":"family-high"},{"legacySlug":"family-max","isMaxMode":true}]},{"name":"max-only","supportsNonMaxMode":false},{"name":"chat","supportsAgent":false}]}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	catalog, err := client.EditorCatalog(context.Background(), "synthetic-token")
	require.NoError(t, err)
	require.Len(t, catalog.Models, 2)
	require.Equal(t, []string{"family", "family-high"}, catalog.Models[0].Slugs)
	require.True(t, catalog.Models[0].SupportsAgent)
	require.False(t, catalog.Models[1].SupportsAgent)
	require.Equal(t, int64(200000), catalog.ContextLengths["family-high"])
	require.NotContains(t, catalog.ContextLengths, "family-max")
}

func Test_ModelContextLengths_rejects_invalid_metadata(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		status int
		body   string
	}{
		{"http_failure", http.StatusServiceUnavailable, `{"error":"private upstream detail"}`},
		{"malformed_json", http.StatusOK, `{"models":`},
		{"invalid_type", http.StatusOK, `{"models":[{"name":"bad","contextTokenLimit":"256k"}]}`},
		{"oversized", http.StatusOK, strings.Repeat(" ", maxModelResponseBytes+1)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Given
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(fixture.status)
				_, err := w.Write([]byte(fixture.body))
				require.NoError(t, err)
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
			require.NoError(t, err)

			// When
			limits, err := client.ModelContextLengths(context.Background(), "token")

			// Then
			require.Error(t, err)
			require.Nil(t, limits)
			require.NotContains(t, err.Error(), "private upstream detail")
		})
	}
}
