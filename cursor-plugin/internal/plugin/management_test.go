package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"cursorplugin/internal/cursorauth"

	"github.com/stretchr/testify/require"
)

func Test_Handler_ManagementStatus_reports_models_disabled_rules_and_honest_quota_state(t *testing.T) {
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:    "secret-access",
		RefreshToken:   "secret-refresh",
		Type:           "cursor",
		Email:          "owner@example.test",
		DisabledModels: []string{"gpt-5"},
	})
	require.NoError(t, err)
	host := &fakeHostCaller{credentialJSON: credentials}
	handler := NewHandler(Dependencies{
		Cursor: fakeModelCursorClient{models: []string{"auto", "gpt-5"}},
		Host:   host,
	})

	response, err := handler.managementStatus(context.Background())

	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	require.Contains(t, string(response.Body), `"subscription_quota":{"status":"unavailable"`)
	require.Contains(t, string(response.Body), `"reason":"Cursor does not publish a subscription remaining-quota API"`)
	require.Contains(t, string(response.Body), `"id":"gpt-5","disabled":true`)
	require.NotContains(t, string(response.Body), "secret-access")
	require.NotContains(t, string(response.Body), "secret-refresh")
}

func Test_Handler_UpdateDisabledModels_persists_rules_in_cursor_auth_without_losing_credentials(t *testing.T) {
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:  "secret-access",
		RefreshToken: "secret-refresh",
		Type:         "cursor",
	})
	require.NoError(t, err)
	host := &fakeHostCaller{credentialJSON: credentials}
	handler := NewHandler(Dependencies{
		Cursor: fakeModelCursorClient{models: []string{"auto", "gpt-5"}},
		Host:   host,
	})
	body := []byte(`{"auth_index":"cursor-auth","disabled_models":["cursor/gpt-5"]}`)

	response, err := handler.updateDisabledModels(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	var saved cursorauth.Credentials
	require.NoError(t, json.Unmarshal(host.savedJSON, &saved))
	require.Equal(t, "secret-access", saved.AccessToken)
	require.Equal(t, "secret-refresh", saved.RefreshToken)
	require.Equal(t, []string{"gpt-5"}, saved.DisabledModels)
	require.Equal(t, "cursor-auth.json", host.savedName)
}

func Test_Handler_ManagementStatus_includes_plugin_local_estimated_usage(t *testing.T) {
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:  "secret-access",
		RefreshToken: "secret-refresh",
		Type:         "cursor",
	})
	require.NoError(t, err)
	handler := NewHandler(Dependencies{
		Cursor: fakeModelCursorClient{models: []string{"auto"}},
		Host:   &fakeHostCaller{credentialJSON: credentials},
	})
	_, err = handler.handleUsage([]byte(`{"Provider":"cursor","AuthIndex":"cursor-auth","Detail":{"InputTokens":12,"OutputTokens":8,"TotalTokens":20}}`))
	require.NoError(t, err)

	response, err := handler.managementStatus(context.Background())

	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"local_usage":{"scope":"plugin_process"`)
	require.Contains(t, string(response.Body), `"estimated":true`)
	require.Contains(t, string(response.Body), `"total_tokens":20`)
}

func Test_Handler_ManagementResource_serves_static_shell_without_exposing_auth_data(t *testing.T) {
	handler := NewHandler(Dependencies{})
	rawRequest, err := json.Marshal(managementRequest{
		Method: "GET",
		Path:   "/v0/resource/plugins/cursor/status",
	})
	require.NoError(t, err)

	result, err := handler.handleManagement(context.Background(), rawRequest)

	require.NoError(t, err)
	response := result.(managementResponse)
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, "text/html; charset=utf-8", response.Headers.Get("content-type"))
	require.Contains(t, string(response.Body), "Cursor 管理")
	require.Contains(t, string(response.Body), "加载状态")
	require.Contains(t, string(response.Body), `createElement("wbr")`)
	require.NotContains(t, string(response.Body), "overflow-wrap: anywhere")
	require.NotContains(t, string(response.Body), "access_token")
}

type fakeHostCaller struct {
	credentialJSON json.RawMessage
	savedJSON      json.RawMessage
	savedName      string
}

func (host *fakeHostCaller) Call(_ context.Context, method string, request any) (json.RawMessage, error) {
	switch method {
	case "host.auth.list":
		return json.RawMessage(`{"files":[{"auth_index":"cursor-auth","name":"cursor-auth.json","type":"cursor","provider":"cursor","label":"owner@example.test","status":"active","success":3,"failed":1}]}`), nil
	case "host.auth.get":
		return json.RawMessage(`{"auth_index":"cursor-auth","name":"cursor-auth.json","json":` + string(host.credentialJSON) + `}`), nil
	case "host.auth.save":
		raw, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		var payload struct {
			Name string          `json:"name"`
			JSON json.RawMessage `json:"json"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		host.savedName = payload.Name
		host.savedJSON = append(json.RawMessage(nil), payload.JSON...)
		return json.RawMessage(`{"name":"cursor-auth.json","path":"/auth/cursor-auth.json"}`), nil
	default:
		return nil, nil
	}
}
