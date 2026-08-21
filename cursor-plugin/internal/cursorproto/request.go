package cursorproto

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RunRequest struct {
	ConversationID string
	MessageID      string
	Model          string
	System         string
	Prompt         string
	TimeZone       string
}

func EncodeRunRequest(request RunRequest) ([]byte, error) {
	if request.ConversationID == "" || request.MessageID == "" || request.Model == "" || request.Prompt == "" {
		return nil, fmt.Errorf("Cursor run request requires conversation, message, model, and prompt")
	}
	client, err := newMessage("AgentClientMessage")
	if err != nil {
		return nil, err
	}
	run, err := newMessage("AgentRunRequest")
	if err != nil {
		return nil, err
	}
	if err := populateRunRequest(run, request); err != nil {
		return nil, err
	}
	if err := setMessage(client, "run_request", run); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(client)
	if err != nil {
		return nil, fmt.Errorf("encode Cursor run request: %w", err)
	}
	return raw, nil
}

func populateRunRequest(run protoreflect.Message, request RunRequest) error {
	state, err := nestedMessage(run, "conversation_state")
	if err != nil {
		return err
	}
	action, err := buildAction(run, request)
	if err != nil {
		return err
	}
	model, err := buildModel(run, request.Model)
	if err != nil {
		return err
	}
	if err := setMessage(run, "conversation_state", state); err != nil {
		return err
	}
	if err := setMessage(run, "action", action); err != nil {
		return err
	}
	if err := setMessage(run, "model_details", model); err != nil {
		return err
	}
	if err := setString(run, "conversation_id", request.ConversationID); err != nil {
		return err
	}
	return nil
}

func buildAction(run protoreflect.Message, request RunRequest) (protoreflect.Message, error) {
	action, err := nestedMessage(run, "action")
	if err != nil {
		return nil, err
	}
	userAction, err := nestedMessage(action, "user_message_action")
	if err != nil {
		return nil, err
	}
	userMessage, err := nestedMessage(userAction, "user_message")
	if err != nil {
		return nil, err
	}
	prompt := request.Prompt
	if request.System != "" {
		prompt = request.System + "\n\n" + prompt
	}
	if err := setString(userMessage, "text", prompt); err != nil {
		return nil, err
	}
	if err := setString(userMessage, "message_id", request.MessageID); err != nil {
		return nil, err
	}
	if err := setMessage(userAction, "user_message", userMessage); err != nil {
		return nil, err
	}
	requestContext, err := nestedMessage(userAction, "request_context")
	if err != nil {
		return nil, err
	}
	environment, err := nestedMessage(requestContext, "env")
	if err != nil {
		return nil, err
	}
	timeZone := request.TimeZone
	if timeZone == "" {
		timeZone = "UTC"
	}
	if err := setString(environment, "time_zone", timeZone); err != nil {
		return nil, err
	}
	if err := setMessage(requestContext, "env", environment); err != nil {
		return nil, err
	}
	if err := setMessage(userAction, "request_context", requestContext); err != nil {
		return nil, err
	}
	if err := setMessage(action, "user_message_action", userAction); err != nil {
		return nil, err
	}
	return action, nil
}

func EncodeClientHeartbeat() ([]byte, error) {
	client, err := newMessage("AgentClientMessage")
	if err != nil {
		return nil, err
	}
	heartbeat, err := nestedMessage(client, "client_heartbeat")
	if err != nil {
		return nil, err
	}
	if err := setMessage(client, "client_heartbeat", heartbeat); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(client)
	if err != nil {
		return nil, fmt.Errorf("encode Cursor client heartbeat: %w", err)
	}
	return raw, nil
}

func buildModel(run protoreflect.Message, modelID string) (protoreflect.Message, error) {
	model, err := nestedMessage(run, "model_details")
	if err != nil {
		return nil, err
	}
	for _, name := range []protoreflect.Name{"model_id", "display_model_id", "display_name", "display_name_short"} {
		if err := setString(model, name, modelID); err != nil {
			return nil, err
		}
	}
	return model, nil
}
