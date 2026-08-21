package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ParseChatRequest_flattens_supported_history(t *testing.T) {
	// Given
	raw := []byte(`{"model":"cursor/auto","stream":true,"messages":[{"role":"system","content":"Be brief."},{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":[{"type":"text","text":"second"}]}]}`)

	// When
	request, err := ParseChatRequest(raw)

	// Then
	require.NoError(t, err)
	require.Equal(t, "auto", request.Model)
	require.Equal(t, "Be brief.", request.System)
	require.Equal(t, "User: first\nAssistant: answer\nUser: second", request.Prompt)
	require.True(t, request.Stream)
}

func Test_StreamChunk_emits_openai_json_payload_for_host_sse_wrapper(t *testing.T) {
	// Given
	turn := NewTurn("cursor/auto")

	// When
	chunk, err := turn.StreamChunk("hello")

	// Then
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(chunk, &payload))
	require.Contains(t, string(payload["choices"]), "hello")
}
