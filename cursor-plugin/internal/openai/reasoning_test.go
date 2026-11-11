package openai

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTurnReasoningCompletion(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		turn := NewTurn("auto")
		if thinking {
			turn.AddReasoning("first ")
			turn.AddReasoning("second")
		}
		turn.AddText("answer")
		raw, err := turn.Completion("prompt")
		require.NoError(t, err)
		var response struct {
			Choices []struct{ Message map[string]any }
		}
		require.NoError(t, json.Unmarshal(raw, &response))
		message := response.Choices[0].Message
		require.Equal(t, "answer", message["content"])
		if thinking {
			require.Equal(t, "first second", message["reasoning_content"])
		} else {
			require.NotContains(t, message, "reasoning_content")
		}
	}
}

func TestTurnReasoningStream(t *testing.T) {
	turn := NewTurn("auto")
	for i, text := range []string{"first", "second", "answer"} {
		var raw []byte
		var err error
		if i < 2 {
			raw, err = turn.StreamReasoning(text)
		} else {
			raw, err = turn.StreamChunk(text)
		}
		require.NoError(t, err)
		var response struct {
			Choices []struct{ Delta map[string]any }
		}
		require.NoError(t, json.Unmarshal(raw, &response))
		delta := response.Choices[0].Delta
		if i < 2 {
			require.Equal(t, text, delta["reasoning_content"])
			require.NotContains(t, delta, "content")
		} else {
			require.Equal(t, text, delta["content"])
			require.NotContains(t, delta, "reasoning_content")
		}
	}
}
