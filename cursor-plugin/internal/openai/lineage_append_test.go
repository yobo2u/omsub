package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_LineageWithAssistant_matches_full_parse_and_preserves_request(t *testing.T) {
	raw := `{"model":"auto","tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}],"messages":[{"role":"system","content":"system"},{"role":"user","content":"question"}]}`
	request, err := ParseChatRequest([]byte(raw))
	require.NoError(t, err)
	original := append([]Digest(nil), request.Lineage.PrefixDigests...)
	for _, response := range []string{"answer", "another\nanswer", "中文结果"} {
		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		extended := strings.TrimSuffix(raw, "]}") + `,{"role":"assistant","content":` + string(encoded) + `}]}`
		expected, err := ParseChatRequest([]byte(extended))
		require.NoError(t, err)

		turn := NewTurn(request.Model)
		turn.AddText(response)
		actual, ok := request.LineageWithAssistant(turn)
		require.True(t, ok)

		require.Equal(t, expected.Lineage, actual)
		require.Equal(t, original, request.Lineage.PrefixDigests)
		require.Len(t, request.Transcript, 2)
	}
}

func Benchmark_LineageWithAssistant_long_cached_prefix(b *testing.B) {
	prompt, err := json.Marshal(strings.Repeat("synthetic context\n", 65536))
	if err != nil {
		b.Fatal(err)
	}
	request, err := ParseChatRequest([]byte(`{"model":"auto","messages":[{"role":"user","content":` + string(prompt) + `}]}`))
	if err != nil {
		b.Fatal(err)
	}
	turn := NewTurn(request.Model)
	turn.AddText("answer")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if lineage, ok := request.LineageWithAssistant(turn); !ok || len(lineage.PrefixDigests) != 2 {
			b.Fatal("missing assistant lineage")
		}
	}
}

func Test_LineageWithAssistant_includes_parallel_tool_calls(t *testing.T) {
	raw := `{"model":"auto","tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"question"}]}`
	request, err := ParseChatRequest([]byte(raw))
	require.NoError(t, err)
	turn := NewTurn("auto")
	turn.AddToolCall("one", "read", `{}`)
	turn.AddToolCall("two", "read", `{}`)
	completion, err := turn.Completion("question")
	require.NoError(t, err)
	var response struct {
		Choices []struct{ Message json.RawMessage }
	}
	require.NoError(t, json.Unmarshal(completion, &response))
	extended := strings.TrimSuffix(raw, "]}") + "," + string(response.Choices[0].Message) + "]}"
	parsed, err := ParseChatRequest([]byte(extended))
	require.NoError(t, err)
	lineage, ok := request.LineageWithAssistant(turn)
	require.True(t, ok)
	require.Equal(t, parsed.Lineage, lineage)
}
