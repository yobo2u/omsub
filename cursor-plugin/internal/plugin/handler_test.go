package plugin

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorauth"
	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func Test_Handler_Register_declares_cursor_auth_models_and_executor(t *testing.T) {
	// Given
	handler := NewHandler(Dependencies{})

	// When
	raw := handler.Call(context.Background(), "plugin.register", []byte(`{"schema_version":3}`))

	// Then
	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.Contains(t, string(response.Result), `"auth_provider":true`)
	require.Contains(t, string(response.Result), `"model_provider":true`)
	require.Contains(t, string(response.Result), `"executor":true`)
	require.Contains(t, string(response.Result), `"management_api":true`)
	require.Contains(t, string(response.Result), `"usage_plugin":true`)
	require.Contains(t, string(response.Result), `"Version":"0.4.1"`)
	require.Contains(t, string(response.Result), `"GitHubRepository":"https://github.com/yobo2u/omsub"`)
}

func Test_Handler_ManagementRegister_exposes_cursor_management_resource_and_authenticated_routes(t *testing.T) {
	handler := NewHandler(Dependencies{})

	raw := handler.Call(context.Background(), "management.register", nil)

	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.Contains(t, string(response.Result), `"Path":"/status"`)
	require.Contains(t, string(response.Result), `"Menu":"Cursor"`)
	require.Contains(t, string(response.Result), `Cursor status, usage and model controls / Cursor 状态、用量与模型管理。`)
	require.Contains(t, string(response.Result), `"Path":"/plugins/cursor/status"`)
	require.Contains(t, string(response.Result), `"Path":"/plugins/cursor/disabled-models"`)
}

func Test_Handler_ExecuteStream_emits_openai_chunks_and_closes(t *testing.T) {
	// Given
	emitter := &captureEmitter{done: make(chan struct{})}
	handler := NewHandler(Dependencies{Cursor: fakeCursorClient{}, Emitter: emitter})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Type:         "cursor",
	})
	require.NoError(t, err)
	request := executorRequest{
		StreamID:    "stream-1",
		StorageJSON: credentials,
		Payload:     []byte(`{"model":"cursor/auto","stream":true,"messages":[{"role":"user","content":"say ok"}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	// When
	raw := handler.Call(context.Background(), "executor.execute_stream", rawRequest)
	<-emitter.done

	// Then
	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.NoError(t, emitter.closeError)
	require.Len(t, emitter.payloads, 3)
	require.Contains(t, string(emitter.payloads[0]), "cursor-plugin-ok")
	require.Equal(t, "[DONE]", string(emitter.payloads[2]))
}

func Test_Handler_Execute_returns_bad_request_for_unsupported_tools(t *testing.T) {
	// Given
	handler := NewHandler(Dependencies{Cursor: fakeCursorClient{}})
	request := executorRequest{
		Payload: []byte(`{"model":"cursor/auto","messages":[{"role":"user","content":"call a tool"}],"tools":[{"type":"function","function":{"name":"noop"}}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	// When
	raw := handler.Call(context.Background(), "executor.execute", rawRequest)

	// Then
	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.False(t, response.OK)
	require.Equal(t, 400, response.Error.HTTPStatus)
}

func Test_Handler_Execute_rejects_model_disabled_by_cursor_plugin(t *testing.T) {
	handler := NewHandler(Dependencies{Cursor: fakeCursorClient{}})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:    "access",
		RefreshToken:   "refresh",
		Type:           "cursor",
		DisabledModels: []string{"gpt-5"},
	})
	require.NoError(t, err)
	request := executorRequest{
		StorageJSON: credentials,
		Payload:     []byte(`{"model":"cursor/gpt-5","messages":[{"role":"user","content":"hello"}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	raw := handler.Call(context.Background(), "executor.execute", rawRequest)

	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.False(t, response.OK)
	require.Equal(t, 400, response.Error.HTTPStatus)
	require.Contains(t, response.Error.Message, "disabled")
}

type fakeCursorClient struct{}

func (fakeCursorClient) Run(_ context.Context, _ cursorapi.RunInput, emit func(cursorproto.ServerEvent) error) error {
	if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventText, Text: "cursor-plugin-ok"}); err != nil {
		return err
	}
	return emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone})
}

func (fakeCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return []string{"auto"}, nil
}

type captureEmitter struct {
	mu         sync.Mutex
	payloads   [][]byte
	closeError error
	done       chan struct{}
}

func (emitter *captureEmitter) Emit(_ context.Context, _ string, payload []byte) error {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	emitter.payloads = append(emitter.payloads, append([]byte(nil), payload...))
	return nil
}

func (emitter *captureEmitter) Close(_ string, err error) error {
	emitter.mu.Lock()
	emitter.closeError = err
	emitter.mu.Unlock()
	close(emitter.done)
	return nil
}
