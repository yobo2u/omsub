package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid OpenAI chat request")

type InvalidRequestError struct {
	Message string
}

func (err *InvalidRequestError) Error() string {
	return err.Message
}

func (err *InvalidRequestError) Is(target error) bool {
	return target == ErrInvalidRequest
}

func invalidRequest(message string) error {
	return &InvalidRequestError{Message: message}
}

type ChatRequest struct {
	Model  string
	System string
	Prompt string
	Stream bool
}

type wireChatRequest struct {
	Model    string          `json:"model"`
	Messages []wireMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    json.RawMessage `json:"tools"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ParseChatRequest(raw []byte) (ChatRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire wireChatRequest
	if err := decoder.Decode(&wire); err != nil {
		return ChatRequest{}, invalidRequest("decode OpenAI chat request: " + err.Error())
	}
	model := strings.TrimPrefix(strings.TrimSpace(wire.Model), "cursor/")
	if model == "" || len(wire.Messages) == 0 {
		return ChatRequest{}, invalidRequest("OpenAI chat request requires model and messages")
	}
	if len(wire.Tools) > 0 && string(wire.Tools) != "null" && string(wire.Tools) != "[]" {
		return ChatRequest{}, invalidRequest("Cursor plugin does not support tools")
	}
	system := make([]string, 0, 2)
	history := make([]string, 0, len(wire.Messages))
	for _, message := range wire.Messages {
		text, err := decodeContent(message.Content)
		if err != nil {
			return ChatRequest{}, err
		}
		switch message.Role {
		case "system", "developer":
			system = append(system, text)
		case "user":
			history = append(history, "User: "+text)
		case "assistant":
			history = append(history, "Assistant: "+text)
		default:
			return ChatRequest{}, invalidRequest(fmt.Sprintf("unsupported OpenAI message role %q", message.Role))
		}
	}
	if len(history) == 0 {
		return ChatRequest{}, invalidRequest("OpenAI chat request requires a user message")
	}
	return ChatRequest{
		Model:  model,
		System: strings.Join(system, "\n\n"),
		Prompt: strings.Join(history, "\n"),
		Stream: wire.Stream,
	}, nil
}

func decodeContent(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return "", invalidRequest("OpenAI message content is empty")
		}
		return text, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", invalidRequest("OpenAI message content must be text")
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" || part.Text == "" {
			return "", invalidRequest("Cursor plugin supports text content only")
		}
		texts = append(texts, part.Text)
	}
	if len(texts) == 0 {
		return "", invalidRequest("OpenAI message content is empty")
	}
	return strings.Join(texts, "\n"), nil
}
