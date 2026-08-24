package openai

import (
	"crypto/sha256"
	"encoding/hex"
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
	unicodeLineID := "call-550e8400-e29b-41d4-a716-446655440000-0\u2028fc_550e8400-e29b-41d4-a716-446655440000_0"
	overlongID := "call-" + strings.Repeat("x", 80)
	ids := []string{legalID, joinedID, collidingJoinedID, unicodeLineID, overlongID}
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
	require.Equal(t, expectedStableToolCallID(joinedID), calls[1].ID)
	require.Equal(t, expectedStableToolCallID(collidingJoinedID), calls[2].ID)
	require.Equal(t, expectedStableToolCallID(unicodeLineID), calls[3].ID)

	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		require.NotEmpty(t, call.ID)
		require.LessOrEqual(t, len(call.ID), 64)
		require.Equal(t, -1, strings.IndexFunc(call.ID, isToolCallIDLineBreak))
		_, duplicate := seen[call.ID]
		require.False(t, duplicate, "tool call IDs must be unique within one turn")
		seen[call.ID] = struct{}{}
	}
}

func Test_Turn_tool_call_id_normalization_is_independent_of_arrival_order(t *testing.T) {
	type input struct {
		id   string
		name string
	}
	legalID := "call-550e8400-e29b-41d4-a716-446655440000-0"
	joinedID := legalID + "\nfc_550e8400-e29b-41d4-a716-446655440000_0"
	forward := []input{{id: legalID, name: "legal"}, {id: joinedID, name: "joined"}}
	reverse := []input{forward[1], forward[0]}
	normalized := func(inputs []input) map[string]string {
		turn := NewTurn("cursor/auto")
		for _, item := range inputs {
			turn.AddToolCall(item.id, item.name, `{}`)
		}
		payload, err := turn.Completion("prompt")
		require.NoError(t, err)
		var response struct {
			Choices []struct {
				Message struct {
					ToolCalls []responseToolCall `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(payload, &response))
		ids := make(map[string]string, len(inputs))
		for _, call := range response.Choices[0].Message.ToolCalls {
			ids[call.Function.Name] = call.ID
		}
		return ids
	}

	forwardIDs := normalized(forward)
	reverseIDs := normalized(reverse)

	require.Equal(t, forwardIDs, reverseIDs)
	require.Equal(t, legalID, forwardIDs["legal"])
	require.Equal(t, expectedStableToolCallID(joinedID), forwardIDs["joined"])
}

func Test_Turn_rejects_blank_tool_call_id(t *testing.T) {
	for _, id := range []string{"", "   ", "\u2028"} {
		turn := NewTurn("cursor/auto")

		turn.AddToolCall(id, "read_file", `{}`)
		_, err := turn.Completion("prompt")

		require.ErrorContains(t, err, "tool call id")
	}
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

func Test_Turn_rejects_whitespace_only_completion_without_tool_calls(t *testing.T) {
	turn := NewTurn("cursor/auto")
	turn.AddText(" \t\n\u2028")

	_, err := turn.Completion("prompt")

	require.ErrorContains(t, err, "no text or tool calls")
}

func isToolCallIDLineBreak(character rune) bool {
	return unicode.IsControl(character) || unicode.In(character, unicode.Zl, unicode.Zp)
}

func expectedStableToolCallID(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return "call_cursor_" + hex.EncodeToString(digest[:26])
}
