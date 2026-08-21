package cursorproto

import (
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
	EventDone     EventKind = "done"
)

type ServerEvent struct {
	Kind   EventKind
	Type   string
	Text   string
	Tokens int
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

func decodeInteraction(interaction protoreflect.Message) (ServerEvent, error) {
	for _, name := range []protoreflect.Name{"text_delta", "thinking_delta", "token_delta", "turn_ended"} {
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
	default:
		return ServerEvent{Kind: EventIgnored, Type: string(name)}, nil
	}
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
