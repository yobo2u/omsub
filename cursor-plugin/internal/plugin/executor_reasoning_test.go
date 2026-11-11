package plugin

import (
	"context"
	"cursorplugin/internal/cursorproto"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutorReasoning(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, limit := range []int{0, 4, 5} {
			t.Run(fmt.Sprintf("stream=%v/limit=%d", stream, limit), func(t *testing.T) {
				step := successfulTextStep("answer", "conversation", nil)
				step.events = append([]cursorproto.ServerEvent{{Kind: cursorproto.EventThinking, Text: "ab"}, {Kind: cursorproto.EventThinking, Text: "cd"}}, step.events...)
				client := &recordingCursorClient{steps: []cursorRunStep{step}}
				emitter := &captureEmitter{done: make(chan struct{})}
				handler := NewHandler(Dependencies{Cursor: client, Emitter: emitter})
				request := contextRequestFixture(t, map[string]any{"model": "cursor/auto", "messages": []map[string]any{textMessage("user", "hello")}, "max_tokens": limit}, false)
				// max_tokens=0 is invalid; omit the optional limit in that case.
				if limit == 0 {
					request = contextRequestFixture(t, map[string]any{"model": "cursor/auto", "messages": []map[string]any{textMessage("user", "hello")}}, false)
				}
				method := "executor.execute"
				if stream {
					method = "executor.execute_stream"
				}
				raw, ok := handler.CallWithStatus(context.Background(), method, request)
				require.True(t, ok, string(raw))
				var payloads [][]byte
				if stream {
					<-emitter.done
					require.NoError(t, emitter.closeError)
					payloads = emitter.payloads
				} else {
					var response struct{ Result executorResponse }
					require.NoError(t, json.Unmarshal(raw, &response))
					payloads = [][]byte{response.Result.Payload}
				}
				reasoning, content, finish := "", "", ""
				for _, payload := range payloads {
					var response struct {
						Choices []struct {
							Message map[string]any
							Delta   map[string]any
							Finish  string `json:"finish_reason"`
						}
					}
					if json.Unmarshal(payload, &response) != nil || len(response.Choices) == 0 {
						continue
					}
					choice := response.Choices[0]
					message := choice.Message
					if stream {
						message = choice.Delta
					}
					if v, ok := message["reasoning_content"].(string); ok {
						reasoning += v
					}
					if v, ok := message["content"].(string); ok {
						content += v
					}
					if choice.Finish != "" {
						finish = choice.Finish
					}
				}
				require.Equal(t, "abcd", reasoning)
				if limit == 0 {
					require.Equal(t, "answer", content)
					require.Equal(t, "stop", finish)
				} else {
					require.Equal(t, "length", finish)
					if limit == 4 {
						require.Empty(t, content)
					} else {
						require.Equal(t, "a", content)
					}
				}
			})
		}
	}
}
