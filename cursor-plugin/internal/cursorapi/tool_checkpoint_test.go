package cursorapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func Test_Client_Run_finishes_tool_without_checkpoint_when_drain_expires(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		readConnectPayload(t, request.Body)
		_, _ = writer.Write(connectFrame(mcpExecServerMessage(7, "call_1", "read_file", "path", "probe.txt")))
		multiple := append(pendingCheckpoint("call_1"), pendingCheckpoint("call_2")...)
		_, _ = writer.Write(connectFrame(checkpointServerMessage(multiple)))
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var toolAt time.Time

	result, err := client.Run(ctx, validRunInput(), func(event cursorproto.ServerEvent) error {
		if event.Kind == cursorproto.EventToolCall {
			toolAt = time.Now()
		}
		return nil
	})

	require.NoError(t, err)
	require.True(t, result.ToolExposed)
	require.Empty(t, result.Checkpoint)
	require.False(t, toolAt.IsZero())
	require.GreaterOrEqual(t, time.Since(toolAt), toolCheckpointDrain)
}

func pendingCheckpoint(id string) []byte {
	pending := fmt.Sprintf(`{"content":[{"type":"tool-call","toolCallId":%q}]}`, id)
	return protowire.AppendString(protowire.AppendTag(nil, 4, protowire.BytesType), pending)
}

func Test_Client_Run_parallel_tool_checkpoints(t *testing.T) {
	for _, position := range []string{"before", "between", "after", "drain"} {
		t.Run(position, func(t *testing.T) {
			checkpoint := append(pendingCheckpoint("call_1"), pendingCheckpoint("call_2")...)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readConnectPayload(t, r.Body)
				send := func(raw []byte) { _, _ = w.Write(connectFrame(raw)) }
				if position == "before" {
					send(checkpointServerMessage(checkpoint))
				}
				send(mcpExecServerMessage(7, "call_1", "read_file", "path", "one"))
				if position == "between" {
					send(checkpointServerMessage(checkpoint))
				}
				send(mcpExecServerMessage(8, "call_2", "read_file", "path", "two"))
				if position == "after" {
					send(checkpointServerMessage(checkpoint))
				}
				send(turnEndedServerMessage())
				if position == "drain" {
					w.(http.Flusher).Flush()
					time.Sleep(30 * time.Millisecond)
					send(checkpointServerMessage(checkpoint))
				}
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
			require.NoError(t, err)
			var ids []string
			result, err := client.Run(context.Background(), validRunInput(), func(e cursorproto.ServerEvent) error {
				if e.Kind == cursorproto.EventToolCall {
					ids = append(ids, e.ID)
				}
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, []string{"call_1", "call_2"}, ids)
			require.True(t, result.ToolCallSeen)
			require.Equal(t, ids, result.ToolCallIDs)
			require.Equal(t, checkpoint, result.Checkpoint)
		})
	}
}

func Test_Client_Run_ignores_tool_call_after_Done(t *testing.T) {
	for _, toolBefore := range []bool{false, true} {
		t.Run(fmt.Sprint(toolBefore), func(t *testing.T) {
			checkpoint := []byte{0x0a, 0x01, 'b'}
			if toolBefore {
				checkpoint = pendingCheckpoint("call_1")
			}
			body := newChunkBody()
			frames := [][]byte{connectFrame(textServerMessage("answer"))}
			if toolBefore {
				frames = append(frames, connectFrame(mcpExecServerMessage(7, "call_1", "read_file", "path", "one")))
			}
			frames = append(frames, connectFrame(turnEndedServerMessage()), connectFrame(mcpExecServerMessage(8, "late", "read_file", "path", "two")), connectFrame(checkpointServerMessage(checkpoint)))
			body = newChunkBody(frames...)
			client := newTestClient(t, staticResponseTransport(body), Config{})
			var events []cursorproto.ServerEvent
			result, err := client.Run(context.Background(), validRunInput(), func(e cursorproto.ServerEvent) error { events = append(events, e); return nil })
			require.NoError(t, err)
			var ids []string
			for _, e := range events {
				if e.Kind == cursorproto.EventToolCall {
					ids = append(ids, e.ID)
				}
			}
			if toolBefore {
				require.Equal(t, []string{"call_1"}, ids)
			} else {
				require.Empty(t, ids)
			}
			require.Equal(t, ids, result.ToolCallIDs)
			require.Equal(t, checkpoint, result.Checkpoint)
		})
	}
}

func Test_Client_Run_discards_pending_checkpoint_without_exposed_tool_call(t *testing.T) {
	body := newChunkBody(connectFrame(textServerMessage("answer")), connectFrame(turnEndedServerMessage()), connectFrame(checkpointServerMessage(pendingCheckpoint("unexposed"))))
	client := newTestClient(t, staticResponseTransport(body), Config{})
	result, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })
	require.NoError(t, err)
	require.False(t, result.ToolCallSeen)
	require.Empty(t, result.ToolCallIDs)
	require.Empty(t, result.Checkpoint)
}

func Test_Client_Run_waits_for_post_done_text_checkpoint_after_pre_done_checkpoint(t *testing.T) {
	staleCheckpoint := []byte{0x0a, 0x05, 's', 't', 'a', 'l', 'e'}
	finalCheckpoint := []byte{0x0a, 0x05, 'f', 'i', 'n', 'a', 'l'}
	body := newChunkBody(
		connectFrame(checkpointServerMessage(staleCheckpoint)),
		connectFrame(textServerMessage("answer")),
		connectFrame(turnEndedServerMessage()),
		connectFrame(checkpointServerMessage(finalCheckpoint)),
	)
	client := newTestClient(t, staticResponseTransport(body), Config{TrailingDrainTimeout: 100 * time.Millisecond})

	result, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })

	require.NoError(t, err)
	require.False(t, result.ToolCallSeen)
	require.Equal(t, finalCheckpoint, result.Checkpoint)
}

func Test_Client_Run_stops_immediately_once_done_and_matching_tool_checkpoint_seen(t *testing.T) {
	checkpoint := pendingCheckpoint("call_1")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		readConnectPayload(t, request.Body)
		_, _ = writer.Write(connectFrame(mcpExecServerMessage(7, "call_1", "read_file", "path", "probe.txt")))
		_, _ = writer.Write(connectFrame(checkpointServerMessage(checkpoint)))
		_, _ = writer.Write(connectFrame(turnEndedServerMessage()))
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	started := time.Now()

	result, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })

	require.NoError(t, err)
	require.Equal(t, checkpoint, result.Checkpoint)
	require.Less(t, time.Since(started), time.Second)
}

func Test_streamState_armDrain_caps_delay_to_remaining_tool_drain_deadline(t *testing.T) {
	state := &streamState{
		result:        RunResult{ToolCallSeen: true},
		drainDeadline: time.Now().Add(40 * time.Millisecond),
	}
	defer func() {
		if state.drain != nil {
			state.drain.Stop()
		}
	}()

	state.armDrain(toolCheckpointDrain)

	select {
	case <-state.drain.C:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("drain timer exceeded maxToolCheckpointDrain deadline")
	}
}
