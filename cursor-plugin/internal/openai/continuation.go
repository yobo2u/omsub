package openai

import (
	"fmt"
	"strings"
)

type Continuation struct {
	Prompt        string
	Images        []Image
	Attachments   []Attachment
	HasToolResult bool
}

func (request ChatRequest) ContinuationFrom(coveredMessageCount int) (Continuation, bool) {
	if coveredMessageCount < 0 || coveredMessageCount >= len(request.Transcript) {
		return Continuation{}, false
	}
	continuation := Continuation{}
	history := make([]string, 0, len(request.Transcript)-coveredMessageCount)
	for _, message := range request.Transcript[coveredMessageCount:] {
		text, images, attachments := messageContent(message.Content)
		continuation.Images = append(continuation.Images, images...)
		continuation.Attachments = append(continuation.Attachments, attachments...)
		switch message.Role {
		case RoleUser:
			history = append(history, "User: "+text)
		case RoleAssistant:
			if text != "" {
				history = append(history, "Assistant: "+text)
			}
			for _, call := range message.ToolCalls {
				history = append(history, fmt.Sprintf("Assistant tool call %s (%s): %s", call.Name, call.ID, call.Arguments))
			}
		case RoleTool:
			continuation.HasToolResult = true
			history = append(history, fmt.Sprintf("Tool result %s (%s): %s", message.Name, message.ToolCallID, text))
		case RoleSystem, RoleDeveloper:
			return Continuation{}, false
		}
	}
	continuation.Prompt = strings.Join(history, "\n")
	return continuation, continuation.Prompt != "" || len(continuation.Images) > 0 || len(continuation.Attachments) > 0
}

func (request ChatRequest) LineageWithAssistant(text string) Lineage {
	transcript := append([]Message(nil), request.Transcript...)
	transcript = append(transcript, Message{
		Role:    RoleAssistant,
		Content: []ContentPart{{Kind: ContentText, Text: text}},
	})
	return buildLineage(request.Model, request.Tools, transcript)
}

func messageContent(parts []ContentPart) (string, []Image, []Attachment) {
	texts := make([]string, 0, len(parts))
	images := make([]Image, 0)
	attachments := make([]Attachment, 0)
	for _, part := range parts {
		switch part.Kind {
		case ContentText:
			texts = append(texts, part.Text)
		case ContentImage:
			texts = append(texts, "[Attached image: "+part.Image.Name+"]")
			images = append(images, part.Image)
		case ContentAttachment:
			texts = append(texts, "[Attached file: "+part.Attachment.Name+"]")
			attachments = append(attachments, part.Attachment)
		}
	}
	return strings.Join(texts, "\n"), images, attachments
}
