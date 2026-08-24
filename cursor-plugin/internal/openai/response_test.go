package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

func Test_Usage_omits_cached_tokens_when_upstream_value_is_unavailable(t *testing.T) {
	// Given
	turn := NewTurn("cursor/auto")
	turn.AddText("answer")

	// When
	payload, err := turn.Completion("prompt")

	// Then
	require.NoError(t, err)
	require.NotContains(t, string(payload), "cached_tokens")
}

func Test_Usage_estimates_tool_call_output_in_completion_tokens(t *testing.T) {
	turn := NewTurn("cursor/auto")
	turn.AddToolCall("call-1", "search_documents", `{"query":"a deliberately long search query that must contribute to completion usage"}`)

	payload, err := turn.Completion("prompt")

	require.NoError(t, err)
	var response struct {
		Usage Usage `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))
	require.Greater(t, response.Usage.CompletionTokens, 1)
}

func Test_Turn_tool_call_ids_are_single_line_bounded_stable_and_unique(t *testing.T) {
	// Given
	turn := NewTurn("cursor/auto")
	legalID := "call-legal-1"
	joinedID := "call-550e8400-e29b-41d4-a716-446655440000-0\nfc_550e8400-e29b-41d4-a716-446655440000_0"
	collidingJoinedID := "call-550e8400-e29b-41d4-a716-446655440000-0\nfc_6ba7b810-9dad-11d1-80b4-00c04fd430c8_1"
	overlongID := "call-" + strings.Repeat("x", 80)
	ids := []string{legalID, joinedID, collidingJoinedID, overlongID}
	for index := range 48 {
		ids = append(ids, fmt.Sprintf("call-%02d-%s", index, strings.Repeat("x", 80)))
	}
	for _, id := range ids {
		turn.AddToolCall(id, "read_file", `{"path":"a.txt"}`)
	}

	// When
	payload, err := turn.Completion("prompt")

	// Then
	require.NoError(t, err)
	var response struct {
		Choices []struct {
			Message struct {
				ToolCalls []responseToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))
	require.Len(t, response.Choices, 1)
	calls := response.Choices[0].Message.ToolCalls
	require.Len(t, calls, len(ids))
	require.Equal(t, legalID, calls[0].ID)
	require.Equal(t, "call-550e8400-e29b-41d4-a716-446655440000-0", calls[1].ID)

	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		require.NotEmpty(t, call.ID)
		require.LessOrEqual(t, len(call.ID), 64)
		require.Equal(t, -1, strings.IndexFunc(call.ID, unicode.IsControl))
		_, duplicate := seen[call.ID]
		require.False(t, duplicate, "tool call IDs must be unique within one turn")
		seen[call.ID] = struct{}{}
	}
}

func Test_Turn_rejects_empty_tool_call_id(t *testing.T) {
	turn := NewTurn("cursor/auto")

	turn.AddToolCall("", "read_file", `{}`)
	_, err := turn.Completion("prompt")

	require.ErrorContains(t, err, "tool call id")
}

func Test_Turn_rejects_completion_without_text_or_tool_calls(t *testing.T) {
	turn := NewTurn("cursor/auto")

	_, err := turn.Completion("prompt")

	require.ErrorContains(t, err, "no text or tool calls")
}

func Test_Turn_rejects_final_chunk_without_text_or_tool_calls(t *testing.T) {
	turn := NewTurn("cursor/auto")

	_, err := turn.FinalChunk("prompt")

	require.ErrorContains(t, err, "no text or tool calls")
}
