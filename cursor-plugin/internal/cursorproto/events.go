package cursorproto

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type EventKind string

const (
	EventIgnored  EventKind = "ignored"
	EventText     EventKind = "text"
	EventThinking EventKind = "thinking"
	EventTokens   EventKind = "tokens"
	EventToolCall EventKind = "tool_call"
	EventDone     EventKind = "done"
)

type ServerEvent struct {
	Kind      EventKind
	Type      string
	Text      string
	Tokens    int
	ID        string
	Name      string
	Arguments string
}

func DecodeServerEvent(raw []byte) (ServerEvent, error) {
	server, err := newMessage("AgentServerMessage")
	if err != nil {
		return ServerEvent{}, err
	}
	if err := proto.Unmarshal(raw, server); err != nil {
		return ServerEvent{}, fmt.Errorf("decode Cursor server message: %w", err)
	}
	messageOneof := server.Descriptor().Oneofs().ByName("message")
	active := server.WhichOneof(messageOneof)
	if active == nil {
		return ServerEvent{Kind: EventIgnored, Type: "empty"}, nil
	}
	if active.Name() == "exec_server_message" {
		execField, err := requireField(server, "exec_server_message")
		if err != nil {
			return ServerEvent{}, err
		}
		return decodeExecServerMessage(server.Get(execField).Message())
	}
	if active.Name() != "interaction_update" {
		return ServerEvent{Kind: EventIgnored, Type: string(active.Name())}, nil
	}
	interactionField, err := requireField(server, "interaction_update")
	if err != nil {
		return ServerEvent{}, err
	}
	if !server.Has(interactionField) {
		return ServerEvent{Kind: EventIgnored, Type: "interaction_update"}, nil
	}
	return decodeInteraction(server.Get(interactionField).Message())
}

func decodeExecServerMessage(execMessage protoreflect.Message) (ServerEvent, error) {
	active := execMessage.WhichOneof(execMessage.Descriptor().Oneofs().ByName("message"))
	if active == nil || active.Name() != "mcp_args" {
		return ServerEvent{Kind: EventIgnored, Type: "exec_server_message"}, nil
	}
	argsField, err := requireField(execMessage, "mcp_args")
	if err != nil {
		return ServerEvent{}, err
	}
	args := execMessage.Get(argsField).Message()
	providerField, err := requireField(args, "provider_identifier")
	if err != nil {
		return ServerEvent{}, err
	}
	if args.Get(providerField).String() != toolProvider {
		return ServerEvent{Kind: EventIgnored, Type: "exec_server_message.mcp_args"}, nil
	}
	name := args.Get(field(args, "tool_name")).String()
	if name == "" {
		name = args.Get(field(args, "name")).String()
	}
	callID := args.Get(field(args, "tool_call_id")).String()
	if callID == "" {
		callID = fmt.Sprintf("exec_%d", execMessage.Get(field(execMessage, "id")).Uint())
	}
	arguments, err := decodeToolArguments(args)
	if err != nil {
		return ServerEvent{}, err
	}
	if name == "" {
		return ServerEvent{}, fmt.Errorf("Cursor MCP exec requires a tool name")
	}
	return ServerEvent{
		Kind: EventToolCall, Type: "exec_server_message.mcp_args", ID: callID, Name: name, Arguments: arguments,
	}, nil
}

func decodeInteraction(interaction protoreflect.Message) (ServerEvent, error) {
	for _, name := range []protoreflect.Name{"text_delta", "thinking_delta", "token_delta", "tool_call_completed", "turn_ended"} {
		descriptor, err := requireField(interaction, name)
		if err != nil {
			return ServerEvent{}, err
		}
		if !interaction.Has(descriptor) {
			continue
		}
		return eventFromField(name, interaction.Get(descriptor).Message())
	}
	return ServerEvent{Kind: EventIgnored, Type: "interaction_update"}, nil
}

func eventFromField(name protoreflect.Name, message protoreflect.Message) (ServerEvent, error) {
	switch name {
	case "text_delta":
		return stringEvent(EventText, message)
	case "thinking_delta":
		return stringEvent(EventThinking, message)
	case "token_delta":
		descriptor, err := requireField(message, "tokens")
		if err != nil {
			return ServerEvent{}, err
		}
		return ServerEvent{Kind: EventTokens, Tokens: int(message.Get(descriptor).Int())}, nil
	case "turn_ended":
		return ServerEvent{Kind: EventDone, Type: string(name)}, nil
	case "tool_call_completed":
		return completedToolCallEvent(message)
	default:
		return ServerEvent{Kind: EventIgnored, Type: string(name)}, nil
	}
}

func completedToolCallEvent(completed protoreflect.Message) (ServerEvent, error) {
	callIDField, err := requireField(completed, "call_id")
	if err != nil {
		return ServerEvent{}, err
	}
	toolCallField, err := requireField(completed, "tool_call")
	if err != nil {
		return ServerEvent{}, err
	}
	if !completed.Has(toolCallField) {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	toolCall := completed.Get(toolCallField).Message()
	active := toolCall.WhichOneof(toolCall.Descriptor().Oneofs().ByName("tool"))
	if active == nil || active.Name() != "mcp_tool_call" {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	mcpCall := toolCall.Get(active).Message()
	argsField, err := requireField(mcpCall, "args")
	if err != nil {
		return ServerEvent{}, err
	}
	if !mcpCall.Has(argsField) {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	args := mcpCall.Get(argsField).Message()
	providerField, err := requireField(args, "provider_identifier")
	if err != nil {
		return ServerEvent{}, err
	}
	if args.Get(providerField).String() != toolProvider {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	name := args.Get(field(args, "tool_name")).String()
	if name == "" {
		name = args.Get(field(args, "name")).String()
	}
	callID := completed.Get(callIDField).String()
	if callID == "" {
		callID = args.Get(field(args, "tool_call_id")).String()
	}
	arguments, err := decodeToolArguments(args)
	if err != nil {
		return ServerEvent{}, err
	}
	if callID == "" || name == "" {
		return ServerEvent{}, fmt.Errorf("Cursor completed tool call requires id and name")
	}
	return ServerEvent{Kind: EventToolCall, Type: "tool_call_completed", ID: callID, Name: name, Arguments: arguments}, nil
}

func decodeToolArguments(args protoreflect.Message) (string, error) {
	descriptor, err := requireField(args, "args")
	if err != nil {
		return "", err
	}
	decoded := make(map[string]any)
	args.Get(descriptor).Map().Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
		var item any
		if err := json.Unmarshal(value.Bytes(), &item); err != nil {
			item = string(value.Bytes())
		}
		decoded[key.String()] = item
		return true
	})
	raw, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("encode Cursor tool arguments: %w", err)
	}
	return string(raw), nil
}

func stringEvent(kind EventKind, message protoreflect.Message) (ServerEvent, error) {
	descriptor, err := requireField(message, "text")
	if err != nil {
		return ServerEvent{}, err
	}
	return ServerEvent{Kind: kind, Type: string(message.Descriptor().Name()), Text: message.Get(descriptor).String()}, nil
}

func DecodeModels(raw []byte) ([]string, error) {
	response, err := newMessage("GetUsableModelsResponse")
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(raw, response); err != nil {
		return nil, fmt.Errorf("decode Cursor models response: %w", err)
	}
	modelsField, err := requireField(response, "models")
	if err != nil {
		return nil, err
	}
	models := response.Get(modelsField).List()
	ids := make([]string, 0, models.Len())
	for index := range models.Len() {
		model := models.Get(index).Message()
		idField, err := requireField(model, "model_id")
		if err != nil {
			return nil, err
		}
		if id := model.Get(idField).String(); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
