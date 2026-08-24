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
	require.Contains(t, string(response.Result), `"usage_plugin":false`)
	require.Contains(t, string(response.Result), `"request_interceptor":true`)
	require.Contains(t, string(response.Result), `"request_lifecycle_plugin":true`)
	require.Contains(t, string(response.Result), `"Version":"0.5.9"`)
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

func Test_Handler_RequestLifecycle_ignores_non_cursor_auth(t *testing.T) {
	handler := NewHandler(Dependencies{})

	observeRequestAuth(t, handler, "request-codex", "codex-auth-id")
	completeCursorRequest(t, handler, "request-codex", "succeeded")

	usage := handler.usage.snapshot("codex-auth-id")
	require.Zero(t, usage.Requests)
	require.Zero(t, usage.Succeeded)
	require.Zero(t, usage.Failed)
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
		AuthID:      "cursor-auth-id",
		StreamID:    "stream-1",
		StorageJSON: credentials,
		Payload:     []byte(`{"model":"cursor/auto","stream":true,"messages":[{"role":"user","content":"say ok"}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	// When
	observeRequestAuth(t, handler, "request-stream", "cursor-auth-id")
	raw := handler.Call(context.Background(), "executor.execute_stream", rawRequest)
	<-emitter.done
	completeCursorRequest(t, handler, "request-stream", "succeeded")

	// Then
	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.NoError(t, emitter.closeError)
	require.Len(t, emitter.payloads, 3)
	require.Contains(t, string(emitter.payloads[0]), "cursor-plugin-ok")
	require.Equal(t, "[DONE]", string(emitter.payloads[2]))
	usage := handler.usage.snapshot("cursor-auth-id")
	require.EqualValues(t, 1, usage.Requests)
	require.EqualValues(t, 1, usage.Succeeded)
	require.Zero(t, usage.Failed)
}

func Test_Handler_Execute_returns_tool_call_and_forwards_tool_catalog(t *testing.T) {
	// Given
	cursor := &toolCursorClient{id: joinedToolCallID}
	handler := NewHandler(Dependencies{Cursor: cursor})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken: "access", RefreshToken: "refresh", Type: "cursor",
	})
	require.NoError(t, err)
	request := executorRequest{
		StorageJSON: credentials,
		Payload:     []byte(`{"model":"cursor/auto","max_tokens":1000,"messages":[{"role":"user","content":[{"type":"text","text":"call a tool"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	// When
	raw := handler.Call(context.Background(), "executor.execute", rawRequest)

	// Then
	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	var result executorResponse
	require.NoError(t, json.Unmarshal(response.Result, &result))
	require.Contains(t, string(result.Payload), `tool_calls`)
	var completion struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(result.Payload, &completion))
	require.Equal(t, canonicalToolCallID, completion.Choices[0].Message.ToolCalls[0].ID)
	require.Len(t, cursor.input.Tools, 1)
	require.Len(t, cursor.input.Images, 1)
}

func Test_Handler_ExecuteStream_emits_tool_call_and_tool_finish_reason(t *testing.T) {
	emitter := &captureEmitter{done: make(chan struct{})}
	handler := NewHandler(Dependencies{Cursor: &toolCursorClient{id: joinedToolCallID}, Emitter: emitter})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken: "access", RefreshToken: "refresh", Type: "cursor",
	})
	require.NoError(t, err)
	request := executorRequest{
		StreamID: "stream-tool", StorageJSON: credentials,
		Payload: []byte(`{"model":"cursor/auto","stream":true,"messages":[{"role":"user","content":"call a tool"}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	raw := handler.Call(context.Background(), "executor.execute_stream", rawRequest)
	<-emitter.done

	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.NoError(t, emitter.closeError)
	require.Len(t, emitter.payloads, 3)
	require.Contains(t, string(emitter.payloads[0]), `"tool_calls"`)
	require.Contains(t, string(emitter.payloads[0]), `"id":"`+canonicalToolCallID+`"`)
	require.NotContains(t, string(emitter.payloads[0]), `fc_`)
	require.Contains(t, string(emitter.payloads[1]), `"finish_reason":"tool_calls"`)
	require.Equal(t, "[DONE]", string(emitter.payloads[2]))
}

func Test_Handler_Execute_rejects_empty_cursor_completion(t *testing.T) {
	handler := NewHandler(Dependencies{Cursor: emptyCursorClient{}})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken: "access", RefreshToken: "refresh", Type: "cursor",
	})
	require.NoError(t, err)
	request := executorRequest{
		StorageJSON: credentials,
		Payload:     []byte(`{"model":"cursor/auto","messages":[{"role":"user","content":"hello"}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	raw := handler.Call(context.Background(), "executor.execute", rawRequest)

	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.False(t, response.OK)
	require.Contains(t, response.Error.Message, "no text or tool calls")
}

func Test_Handler_ExecuteStream_rejects_empty_cursor_completion(t *testing.T) {
	emitter := &captureEmitter{done: make(chan struct{})}
	handler := NewHandler(Dependencies{Cursor: emptyCursorClient{}, Emitter: emitter})
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken: "access", RefreshToken: "refresh", Type: "cursor",
	})
	require.NoError(t, err)
	request := executorRequest{
		StreamID: "stream-empty", StorageJSON: credentials,
		Payload: []byte(`{"model":"cursor/auto","stream":true,"messages":[{"role":"user","content":"hello"}]}`),
	}
	rawRequest, err := json.Marshal(request)
	require.NoError(t, err)

	raw := handler.Call(context.Background(), "executor.execute_stream", rawRequest)
	<-emitter.done

	var response envelope
	require.NoError(t, json.Unmarshal(raw, &response))
	require.True(t, response.OK)
	require.ErrorContains(t, emitter.closeError, "no text or tool calls")
	require.Empty(t, emitter.payloads)
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

func (fakeCursorClient) Run(_ context.Context, _ cursorapi.RunInput, emit func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventText, Text: "cursor-plugin-ok"}); err != nil {
		return cursorapi.RunResult{OutputExposed: true}, err
	}
	return cursorapi.RunResult{OutputExposed: true}, emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone})
}

type toolCursorClient struct {
	input cursorapi.RunInput
	id    string
}

func (client *toolCursorClient) Run(_ context.Context, input cursorapi.RunInput, emit func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	client.input = input
	id := client.id
	if id == "" {
		id = "call_1"
	}
	err := emit(cursorproto.ServerEvent{
		Kind: cursorproto.EventToolCall, ID: id, Name: "read_file", Arguments: `{"path":"a.txt"}`,
	})
	return cursorapi.RunResult{ToolExposed: true}, err
}

type emptyCursorClient struct{}

func (emptyCursorClient) Run(_ context.Context, _ cursorapi.RunInput, emit func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	return cursorapi.RunResult{}, emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone})
}

func (emptyCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return []string{"auto"}, nil
}

func (*toolCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return []string{"auto"}, nil
}

func (fakeCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return []string{"auto"}, nil
}

const (
	canonicalToolCallID = "call-550e8400-e29b-41d4-a716-446655440000-0"
	joinedToolCallID    = canonicalToolCallID + "\nfc_550e8400-e29b-41d4-a716-446655440000_0"
)

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
