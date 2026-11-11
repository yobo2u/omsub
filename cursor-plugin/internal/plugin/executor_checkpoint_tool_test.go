package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func Test_Checkpoint_reuses_tool_turns_with_HTTP2_transport(t *testing.T) {
	var calls atomic.Int32
	blob := []byte("seed-blob")
	blobID := checkpointTestBytesField(1, []byte("b"))
	seed := strings.Repeat("reference ", 1000)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, err := readCheckpointTestPayload(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		turn := calls.Add(1)
		send := func(payload []byte) {
			_, _ = writer.Write(cursorproto.EncodeConnectFrame(payload))
		}
		if turn == 1 {
			send(checkpointTestBytesField(4, append([]byte{0x08, 0x01, 0x1a, byte(len(blobID) + len(blob) + 2)}, append(blobID, checkpointTestBytesField(2, blob)...)...)))
		} else {
			if bytes.Contains(payload, []byte(seed)) || !bytes.Contains(payload, []byte(fmt.Sprintf("result-%d", turn-1))) {
				t.Error("tool continuation did not send only the new result")
			}
			if !bytes.Contains(payload, checkpointTestBytesField(1, []byte{0xff, 0x01})) ||
				!bytes.Contains(payload, pendingToolMessage(turn-1)) {
				t.Error("tool continuation did not send the matching checkpoint")
			}
			send(checkpointTestBytesField(4, append([]byte{0x08, 0x01}, checkpointTestBytesField(2, blobID)...)))
		}
		writer.(http.Flusher).Flush()
		reply, err := readCheckpointTestPayload(request.Body)
		if err != nil || (turn > 1 && !bytes.Contains(reply, blob)) {
			t.Error("continuation did not return saved blob", err)
			return
		}
		if turn <= 2 {
			args := checkpointTestBytesField(3, []byte(fmt.Sprintf("raw\n%d", turn)))
			args = append(args, checkpointTestBytesField(4, []byte("opencodex-responses"))...)
			args = append(args, checkpointTestBytesField(5, []byte("read"))...)
			if turn == 2 {
				send(checkpointTestBytesField(3, pendingToolCheckpoint(turn)))
			}
			send(checkpointTestBytesField(2, append([]byte{0x08, 0x01}, checkpointTestBytesField(11, args)...)))
			send(checkpointTestBytesField(3, pendingToolCheckpoint(0)))
			if turn == 1 {
				writer.(http.Flusher).Flush()
				// Live Cursor delivered a matching tool checkpoint 180 ms after the call.
				time.Sleep(180 * time.Millisecond)
				send(checkpointTestBytesField(3, pendingToolCheckpoint(turn)))
			}
		} else {
			send(checkpointTestBytesField(1, checkpointTestBytesField(1, checkpointTestBytesField(1, []byte("done")))))
			send([]byte{0x0a, 0x02, 0x72, 0x00})
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := cursorapi.NewClient(cursorapi.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	emitter := &captureEmitter{done: make(chan struct{})}
	handler := NewHandler(Dependencies{Cursor: client, Emitter: emitter})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	history := []map[string]any{textMessage("user", seed)}
	fixture := func(stream string) []byte {
		return executorFixture(t, "session", "account", "auth", "auto", stream, history)
	}
	appendResult := func(assistant map[string]any, result string) {
		assistant["role"] = "assistant"
		tools := assistant["tool_calls"].([]any)
		require.Len(t, tools, 1)
		id := tools[0].(map[string]any)["id"].(string)
		require.True(t, strings.HasPrefix(id, "call_cursor_"))
		history = append(history, assistant, map[string]any{"role": "tool", "tool_call_id": id, "content": result})
	}

	first, err := handler.execute(ctx, fixture(""))
	require.NoError(t, err)
	var response struct {
		Choices []struct{ Message map[string]any }
	}
	require.NoError(t, json.Unmarshal(first.(executorResponse).Payload, &response))
	appendResult(response.Choices[0].Message, "result-1")

	_, err = handler.executeStream(ctx, fixture("stream"))
	require.NoError(t, err)
	<-emitter.done
	require.NoError(t, emitter.closeError)
	var chunks int
	for _, payload := range emitter.payloads {
		var response struct {
			Choices []struct{ Delta map[string]any }
		}
		if json.Unmarshal(payload, &response) == nil && len(response.Choices) == 1 && response.Choices[0].Delta["tool_calls"] != nil {
			appendResult(response.Choices[0].Delta, "result-2")
			chunks++
		}
	}
	require.Equal(t, 1, chunks)
	_, err = handler.execute(ctx, fixture(""))
	require.NoError(t, err)
	require.EqualValues(t, 3, calls.Load())
}

func pendingToolCheckpoint(turn int32) []byte {
	return append(checkpointTestBytesField(1, []byte{0xff, 0x01}), checkpointTestBytesField(4, pendingToolMessage(turn))...)
}

func pendingToolMessage(turn int32) []byte {
	return []byte(fmt.Sprintf(`{"role":"assistant","content":[{"type":"tool-call","toolCallId":"raw\n%d"}]}`, turn))
}

func TestCheckpointParallelToolsResume(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ids    []string
		resume bool
	}{
		{"ordered", []string{"call_1", "call_2"}, true}, {"permuted", []string{"call_2", "call_1"}, true},
		{"missing", []string{"call_1"}, false}, {"wrong", []string{"call_1", "wrong"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkpoint := checkpointTestBytesField(4, []byte(`{"content":[{"type":"tool-call","toolCallId":"call_1"},{"type":"tool-call","toolCallId":"call_2"}]}`))
			first := cursorRunStep{events: []cursorproto.ServerEvent{{Kind: cursorproto.EventToolCall, ID: "call_1", Name: "read", Arguments: "{}"}, {Kind: cursorproto.EventToolCall, ID: "call_2", Name: "read", Arguments: "{}"}}, result: cursorapi.RunResult{ConversationID: "conversation", Checkpoint: checkpoint, ToolExposed: true}}
			client := &recordingCursorClient{steps: []cursorRunStep{first, successfulTextStep("done", "conversation", nil)}}
			emitter := &captureEmitter{done: make(chan struct{})}
			handler := NewHandler(Dependencies{Cursor: client, Emitter: emitter})
			history := []map[string]any{textMessage("user", strings.Repeat("seed ", 100))}
			raw, err := handler.execute(context.Background(), executorFixture(t, "session", "account", "auth", "auto", "", history))
			require.NoError(t, err)
			var response struct {
				Choices []struct{ Message map[string]any }
			}
			require.NoError(t, json.Unmarshal(raw.(executorResponse).Payload, &response))
			assistant := response.Choices[0].Message
			assistant["role"] = "assistant"
			history = append(history, assistant)
			for _, id := range tc.ids {
				history = append(history, map[string]any{"role": "tool", "tool_call_id": id, "content": "result"})
			}
			// Use streaming for resumed turns to verify resumed tool turns finish correctly.
			_, err = handler.executeStream(context.Background(), executorFixture(t, "session", "account", "auth", "auto", "parallel", history))
			require.NoError(t, err)
			<-emitter.done
			require.NoError(t, emitter.closeError)
			mode := cursorproto.FullReplay
			if tc.resume {
				mode = cursorproto.CheckpointSuffix
			}
			require.Equal(t, mode, client.Inputs()[1].Mode)
		})
	}
}
