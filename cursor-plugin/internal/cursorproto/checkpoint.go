package cursorproto

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func decodeCheckpoint(state protoreflect.Message) (ServerEvent, error) {
	checkpoint, err := proto.Marshal(state.Interface())
	if err != nil {
		return ServerEvent{}, fmt.Errorf("encode Cursor conversation checkpoint: %w", err)
	}
	if len(checkpoint) == 0 {
		return ServerEvent{}, fmt.Errorf("decode Cursor conversation checkpoint: empty state")
	}
	return ServerEvent{
		Kind: EventCheckpoint, Type: "conversation_checkpoint_update", Checkpoint: checkpoint,
	}, nil
}

// MatchesPendingTool preserves the single-call checkpoint contract.
func MatchesPendingTool(checkpoint []byte, id string) bool {
	return MatchesPendingTools(checkpoint, []string{id})
}

func MatchesPendingTools(checkpoint []byte, ids []string) bool {
	actual, ok := CheckpointPendingToolCalls(checkpoint)
	return ok && slices.Equal(actual, ids)
}

// CheckpointPendingToolCalls returns all pending call IDs in wire order.
func CheckpointPendingToolCalls(checkpoint []byte) ([]string, bool) {
	run, err := newMessage("AgentRunRequest")
	if err != nil {
		return nil, false
	}
	state, err := conversationState(run, checkpoint)
	if err != nil {
		return nil, false
	}
	pending := state.Get(field(state, "pending_tool_calls")).List()
	messages := make([]string, 0, pending.Len())
	for i := 0; i < pending.Len(); i++ {
		messages = append(messages, pending.Get(i).String())
	}
	var ids []string
	for _, raw := range messages {
		var message struct {
			Content []struct {
				Type string `json:"type"`
				ID   string `json:"toolCallId"`
			} `json:"content"`
		}
		if json.Unmarshal([]byte(raw), &message) != nil {
			return nil, false
		}
		found := false
		for _, part := range message.Content {
			if part.Type != "tool-call" {
				continue
			}
			id := part.ID
			if strings.TrimSpace(id) == "" || slices.Contains(ids, id) {
				return nil, false
			}
			ids = append(ids, id)
			found = true
		}
		if !found {
			return nil, false
		}
	}
	return ids, len(ids) > 0
}

// HasPendingTools rejects malformed state as well as unresolved tool calls.
func HasPendingTools(checkpoint []byte) bool {
	run, err := newMessage("AgentRunRequest")
	if err != nil {
		return true
	}
	state, err := conversationState(run, checkpoint)
	if err != nil {
		return true
	}
	return state.Get(field(state, "pending_tool_calls")).List().Len() != 0
}
