package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const toolResultContinuationPrompt = "Continue: the requested tool results above are complete. Do not repeat a completed tool call; continue with the user's request."

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
	suffix := request.Transcript[coveredMessageCount:]
	if coveredMessageCount > 0 {
		calls := request.Transcript[coveredMessageCount-1].ToolCalls
		if len(calls) > 0 && !matchesPendingResult(calls, suffix) {
			return Continuation{}, false
		}
	}
	continuation := Continuation{}
	history := make([]string, 0, len(request.Transcript)-coveredMessageCount)
	for _, message := range suffix {
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
	history = appendToolResultContinuation(history, request.Transcript)
	continuation.Prompt = strings.Join(history, "\n")
	return continuation, continuation.Prompt != "" || len(continuation.Images) > 0 || len(continuation.Attachments) > 0
}

func matchesPendingResult(calls []ToolCall, suffix []Message) bool {
	if len(calls) == 0 || len(suffix) < len(calls) {
		return false
	}
	expected := make(map[string]string, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" {
			return false
		}
		if _, exists := expected[call.ID]; exists {
			return false
		}
		expected[call.ID] = call.Name
	}
	for _, result := range suffix[:len(calls)] {
		name, ok := expected[result.ToolCallID]
		if !ok || result.Role != RoleTool || result.Name != name {
			return false
		}
		delete(expected, result.ToolCallID)
	}
	for _, message := range suffix[len(calls):] {
		if message.Role != RoleUser {
			return false
		}
	}
	return true
}

func appendToolResultContinuation(history []string, transcript []Message) []string {
	if len(transcript) == 0 || transcript[len(transcript)-1].Role != RoleTool {
		return history
	}
	return append(history, toolResultContinuationPrompt)
}

func (request ChatRequest) LineageWithAssistant(turn *Turn) (Lineage, bool) {
	if turn.validateOutput() != nil {
		return Lineage{}, false
	}
	message := Message{Role: RoleAssistant}
	if turn.text != "" {
		message.Content = []ContentPart{{Kind: ContentText, Text: turn.text}}
	}
	for _, call := range turn.tools {
		message.ToolCalls = append(message.ToolCalls, ToolCall{
			ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments,
		})
	}
	encoded, _ := json.Marshal(canonicalizeMessage(message))
	step := sha256.New()
	step.Write(request.Lineage.tail[:])
	step.Write(encoded)
	next := request.Lineage
	copy(next.tail[:], step.Sum(nil))
	next.PrefixDigests = make([]Digest, len(request.Lineage.PrefixDigests)+1)
	copy(next.PrefixDigests, request.Lineage.PrefixDigests)
	next.PrefixDigests[len(request.Lineage.PrefixDigests)] = Digest(hex.EncodeToString(next.tail[:]))
	return next, true
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
