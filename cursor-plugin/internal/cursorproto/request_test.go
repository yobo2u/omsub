package cursorproto

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func Test_EncodeRunRequest_places_model_and_prompt_in_wire_message(t *testing.T) {
	// Given
	request := RunRequest{
		ConversationID: "cursor_conversation",
		MessageID:      "message-1",
		Model:          "auto",
		System:         "Answer briefly.",
		Prompt:         "Reply with cursor-plugin-ok",
		TimeZone:       "Asia/Shanghai",
	}

	// When
	raw, err := EncodeRunRequest(request)

	// Then
	require.NoError(t, err)
	message, err := newMessage("AgentClientMessage")
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(raw, message))
	run := message.Get(field(message, "run_request")).Message()
	require.Equal(t, "cursor_conversation", run.Get(field(run, "conversation_id")).String())
	require.False(t, run.Has(field(run, "custom_system_prompt")))
	model := run.Get(field(run, "model_details")).Message()
	require.Equal(t, "auto", model.Get(field(model, "model_id")).String())
	action := run.Get(field(run, "action")).Message()
	userAction := action.Get(field(action, "user_message_action")).Message()
	userMessage := userAction.Get(field(userAction, "user_message")).Message()
	require.Equal(t, "Answer briefly.\n\nReply with cursor-plugin-ok", userMessage.Get(field(userMessage, "text")).String())
	requestContext := userAction.Get(field(userAction, "request_context")).Message()
	environment := requestContext.Get(field(requestContext, "env")).Message()
	require.Equal(t, "Asia/Shanghai", environment.Get(field(environment, "time_zone")).String())
}

func Test_DecodeServerEvent_maps_text_and_turn_end(t *testing.T) {
	// Given
	textWire, err := encodeTestInteractionUpdate("text_delta", "text", "hello")
	require.NoError(t, err)
	endWire, err := encodeTestInteractionUpdate("turn_ended", "", "")
	require.NoError(t, err)

	// When
	textEvent, err := DecodeServerEvent(textWire)
	require.NoError(t, err)
	endEvent, err := DecodeServerEvent(endWire)

	// Then
	require.NoError(t, err)
	require.Equal(t, EventText, textEvent.Kind)
	require.Equal(t, "hello", textEvent.Text)
	require.Equal(t, EventDone, endEvent.Kind)
}

func Test_DecodeServerEvent_names_ignored_top_level_message(t *testing.T) {
	// Given
	server, err := newMessage("AgentServerMessage")
	require.NoError(t, err)
	kv, err := nestedMessage(server, "kv_server_message")
	require.NoError(t, err)
	require.NoError(t, setMessage(server, "kv_server_message", kv))
	raw, err := proto.Marshal(server)
	require.NoError(t, err)

	// When
	event, err := DecodeServerEvent(raw)

	// Then
	require.NoError(t, err)
	require.Equal(t, EventIgnored, event.Kind)
	require.Equal(t, "kv_server_message", event.Type)
}

func encodeTestInteractionUpdate(updateName, valueName, value string) ([]byte, error) {
	server, err := newMessage("AgentServerMessage")
	if err != nil {
		return nil, err
	}
	interaction, err := nestedMessage(server, "interaction_update")
	if err != nil {
		return nil, err
	}
	updateField := protoreflect.Name(updateName)
	update, err := nestedMessage(interaction, updateField)
	if err != nil {
		return nil, err
	}
	if valueName != "" {
		if err := setString(update, protoreflect.Name(valueName), value); err != nil {
			return nil, err
		}
	}
	if err := setMessage(interaction, updateField, update); err != nil {
		return nil, err
	}
	if err := setMessage(server, "interaction_update", interaction); err != nil {
		return nil, err
	}
	return proto.Marshal(server)
}
