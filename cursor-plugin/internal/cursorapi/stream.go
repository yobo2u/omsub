package cursorapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"cursorplugin/internal/cursorproto"
)

const connectEndStreamFlag byte = 0x02
const toolCheckpointDrain = 2 * time.Second

type streamState struct {
	result      RunResult
	started     time.Time
	terminal    bool
	contentSeen bool
	doneSent    bool
	doneSeen    bool
	drain       *time.Timer
	eventCounts map[string]int
	queries     queryDiagnostic
	trace       eventTrace
}

func readRunStream(
	ctx context.Context,
	body io.ReadCloser,
	result RunResult,
	emit func(cursorproto.ServerEvent) error,
	blobs *cursorproto.BlobStore,
	imageWrites *cursorproto.ImageWriteExecutor,
	environment cursorproto.RequestEnvironment,
	outbound chan<- []byte,
	watchdogs *runWatchdogs,
	drainDelay time.Duration,
) (RunResult, error) {
	state := streamState{result: result, started: time.Now()}
	defer func() {
		if state.drain != nil {
			state.drain.Stop()
		}
	}()
	decoder := cursorproto.NewFrameDecoder(32 << 20)
	reads := startBodyReader(body)
	defer stopBodyReader(body, reads)
	for {
		read, drainExpired, err := nextBodyRead(ctx, reads, state.drain)
		if err != nil {
			return state.finalResult(), state.annotateProgressTimeout(runCause(ctx, err))
		}
		if drainExpired {
			return state.finalResult(), nil
		}
		if len(read.data) > 0 {
			watchdogs.sawData()
			frames, err := decoder.Push(read.data)
			if err != nil {
				return state.finalResult(), err
			}
			for _, frame := range frames {
				watchdogs.sawFrame()
				finished, err := state.handleFrame(ctx, frame, emit, blobs, imageWrites, environment, outbound, watchdogs, drainDelay)
				if err != nil || finished {
					return state.finalResult(), state.annotateProgressTimeout(err)
				}
			}
		}
		if errors.Is(read.err, io.EOF) {
			if state.terminal {
				return state.finalResult(), nil
			}
			return state.finalResult(), errors.New("Cursor stream closed before turn end")
		}
		if read.err != nil {
			return state.finalResult(), state.annotateProgressTimeout(runCause(ctx, fmt.Errorf("read Cursor stream: %w", read.err)))
		}
	}
}

func (state *streamState) handleFrame(
	ctx context.Context,
	frame cursorproto.Frame,
	emit func(cursorproto.ServerEvent) error,
	blobs *cursorproto.BlobStore,
	imageWrites *cursorproto.ImageWriteExecutor,
	environment cursorproto.RequestEnvironment,
	outbound chan<- []byte,
	watchdogs *runWatchdogs,
	drainDelay time.Duration,
) (bool, error) {
	if frame.Flags == connectEndStreamFlag {
		if err := connectEndStreamError(frame.Payload); err != nil {
			return false, err
		}
		if state.terminal {
			return true, nil
		}
		if !state.contentSeen {
			return false, ErrEmptyCompletion
		}
		if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone, Type: "connect_end_stream"}); err != nil {
			return false, fmt.Errorf("emit Cursor event: %w", err)
		}
		state.terminal = true
		state.doneSent = true
		watchdogs.stop()
		return true, nil
	}
	if frame.Flags != 0 {
		return false, fmt.Errorf("Cursor stream ended with Connect flags %d", frame.Flags)
	}
	reply, handled, err := blobs.HandleServerMessage(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ReplyRequestContext(frame.Payload, environment)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = imageWrites.HandleServerMessage(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ReplyInteractionQuery(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		state.result.InteractionResponded = true
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ReplyNativeReadOnlyExec(frame.Payload, environment.Tools)
	if err != nil {
		return false, err
	}
	if handled {
		state.result.InteractionResponded = true
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	shellReplies, handled, err := cursorproto.ReplyNativeShellExec(frame.Payload, environment.Tools)
	if err != nil {
		return false, err
	}
	if handled {
		state.result.InteractionResponded = true
		for _, shellReply := range shellReplies {
			select {
			case outbound <- shellReply:
				watchdogs.sawProgress()
			case <-ctx.Done():
				return false, context.Cause(ctx)
			}
		}
	}
	event, err := cursorproto.DecodeServerEvent(frame.Payload)
	if err != nil {
		return false, err
	}
	state.recordEvent(event)
	state.queries.record(event, time.Since(state.started))
	if event.Kind == cursorproto.EventCheckpoint {
		if state.result.ToolCallSeen {
			ids, ok := cursorproto.CheckpointPendingToolCalls(event.Checkpoint)
			if !ok || len(ids) < len(state.result.ToolCallIDs) || !slices.Equal(ids[:len(state.result.ToolCallIDs)], state.result.ToolCallIDs) {
				return false, nil
			}
		}

		state.result.Checkpoint = append(state.result.Checkpoint[:0], event.Checkpoint...)
		if !state.result.ToolCallSeen {
			return state.terminal, nil
		}
		if state.drain != nil {
			state.drain.Reset(toolCheckpointDrain)
		}
		return false, nil
	}
	if state.doneSeen || (state.terminal && event.Kind != cursorproto.EventToolCall) {
		if event.Kind == cursorproto.EventDone && !state.doneSent {
			if err := emit(event); err != nil {
				return false, fmt.Errorf("emit Cursor event: %w", err)
			}
			state.doneSent = true
			state.doneSeen = true
			state.result.OutputExposed = true
		}
		return false, nil
	}
	if eventMakesProgress(event.Kind) {
		watchdogs.sawProgress()
		if state.result.TTFT == 0 && event.Kind != cursorproto.EventDone {
			state.result.TTFT = time.Since(state.started)
		}
	}
	state.result.OutputExposed = state.result.OutputExposed || eventExposesOutput(event.Kind)
	state.result.ToolExposed = state.result.ToolExposed || event.Kind == cursorproto.EventToolCall
	state.contentSeen = state.contentSeen || event.Kind == cursorproto.EventText || event.Kind == cursorproto.EventImage
	if err := emit(event); err != nil {
		return false, fmt.Errorf("emit Cursor event: %w", err)
	}
	state.doneSent = state.doneSent || event.Kind == cursorproto.EventDone
	state.doneSeen = state.doneSeen || event.Kind == cursorproto.EventDone
	if event.Kind == cursorproto.EventDone || event.Kind == cursorproto.EventToolCall {
		if state.result.ToolCallSeen {
			drainDelay = toolCheckpointDrain
		}
		state.terminal = true
		watchdogs.stop()
		if event.Kind == cursorproto.EventToolCall {
			state.result.ToolCallSeen = true
			state.result.ToolCallID = event.ID
			state.result.ToolCallIDs = append(state.result.ToolCallIDs, event.ID)
			// Cursor can send the checkpoint for the tool after its exec request.
			drainDelay = toolCheckpointDrain
		}
		if state.drain != nil {
			state.drain.Stop()
		}
		state.drain = time.NewTimer(drainDelay)
	}
	return false, nil
}

// Keep only checkpoints consistent with the complete output exposed to the client.
func (state *streamState) finalResult() RunResult {
	result := state.result
	if result.ToolCallSeen {
		if !cursorproto.MatchesPendingTools(result.Checkpoint, result.ToolCallIDs) {
			result.Checkpoint = nil
		}
	} else if cursorproto.HasPendingTools(result.Checkpoint) {
		result.Checkpoint = nil
	}
	return result
}
