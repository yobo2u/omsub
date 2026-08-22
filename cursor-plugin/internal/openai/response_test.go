package openai

import (
	"testing"

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
