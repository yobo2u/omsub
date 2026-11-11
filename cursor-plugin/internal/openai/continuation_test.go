package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ToolResultContinuation_marks_the_completed_call_for_checkpoint_and_full_replay(t *testing.T) {
	raw := []byte(`{
		"model":"cursor/grok-4.6",
		"messages":[
			{"role":"user","content":"inspect the project"},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"path\":\"README.md\"}"}}]},
			{"role":"tool","tool_call_id":"call-1","content":"project contents"}
		]
	}`)
	request, err := ParseChatRequest(raw)
	require.NoError(t, err)

	continuation, ok := request.ContinuationFrom(1)

	require.True(t, ok)
	require.True(t, continuation.HasToolResult)
	require.True(t, strings.HasSuffix(continuation.Prompt, toolResultContinuationPrompt))
	require.True(t, strings.HasSuffix(request.Prompt, toolResultContinuationPrompt))
}

func Test_Continuation_without_trailing_tool_result_is_unchanged(t *testing.T) {
	request := ChatRequest{Transcript: []Message{{
		Role: RoleUser, Content: []ContentPart{{Kind: ContentText, Text: "next"}},
	}}}

	continuation, ok := request.ContinuationFrom(0)

	require.True(t, ok)
	require.Equal(t, "User: next", continuation.Prompt)
}

func Test_ContinuationFrom_requires_the_pending_tool_result(t *testing.T) {
	pending := Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-1", Name: "read"}}}
	result := Message{Role: RoleTool, ToolCallID: "call-1", Name: "read"}
	for _, tc := range []struct {
		name   string
		suffix []Message
		want   bool
	}{
		{name: "matching", suffix: []Message{result}, want: true},
		{name: "missing", suffix: []Message{{Role: RoleUser}}},
		{name: "wrong id", suffix: []Message{{Role: RoleTool, ToolCallID: "other", Name: "read"}}},
		{name: "wrong name", suffix: []Message{{Role: RoleTool, ToolCallID: "call-1", Name: "write"}}},
		{name: "duplicate", suffix: []Message{result, result}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := ChatRequest{Transcript: append([]Message{pending}, tc.suffix...)}
			_, ok := request.ContinuationFrom(1)
			require.Equal(t, tc.want, ok)
		})
	}
}

func TestParallelToolContinuation(t *testing.T) {
	calls := []ToolCall{{ID: "call_1", Name: "read"}, {ID: "call_2", Name: "read"}}
	one := Message{Role: RoleTool, ToolCallID: "call_1", Name: "read"}
	two := Message{Role: RoleTool, ToolCallID: "call_2", Name: "read"}
	for _, tc := range []struct {
		name   string
		calls  []ToolCall
		suffix []Message
		want   bool
	}{
		{"ordered", calls, []Message{one, two}, true},
		{"permuted", calls, []Message{two, one}, true},
		{"user tail", calls, []Message{two, one, {Role: RoleUser}}, true},
		{"missing", calls, []Message{one}, false},
		{"wrong", calls, []Message{one, {Role: RoleTool, ToolCallID: "wrong", Name: "read"}}, false},
		{"duplicate result", calls, []Message{one, one}, false},
		{"duplicate pending", []ToolCall{calls[0], calls[0]}, []Message{one, two}, false},
		{"empty pending", []ToolCall{{Name: "read"}, calls[1]}, []Message{one, two}, false},
		{"assistant tail", calls, []Message{one, two, {Role: RoleAssistant}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := ChatRequest{Transcript: append([]Message{{Role: RoleAssistant, ToolCalls: tc.calls}}, tc.suffix...)}
			_, ok := request.ContinuationFrom(1)
			require.Equal(t, tc.want, ok)
		})
	}
}
