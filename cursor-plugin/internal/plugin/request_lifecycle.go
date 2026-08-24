package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type requestInterceptRequest struct {
	RequestID string                     `json:"RequestID"`
	Headers   http.Header                `json:"Headers"`
	Body      []byte                     `json:"Body"`
	Metadata  map[string]json.RawMessage `json:"Metadata"`
}

type requestInterceptResponse struct {
	Headers http.Header `json:"Headers,omitempty"`
	Body    []byte      `json:"Body,omitempty"`
}

type requestCompletion struct {
	RequestID string `json:"RequestID"`
	Outcome   string `json:"Outcome"`
}

func (handler *Handler) passRequest(raw []byte) (any, error) {
	request, err := decodeRequestIntercept(raw)
	if err != nil {
		return nil, err
	}
	return requestInterceptResponse{Headers: request.Headers, Body: request.Body}, nil
}

func (handler *Handler) observeRequestAuth(raw []byte) (any, error) {
	request, err := decodeRequestIntercept(raw)
	if err != nil {
		return nil, err
	}
	authID := metadataString(request.Metadata, "selected_auth_id")
	if !strings.HasPrefix(strings.ToLower(authID), "cursor-") {
		authID = ""
	}
	handler.usage.selectRequestAuth(request.RequestID, authID)
	return requestInterceptResponse{Headers: request.Headers, Body: request.Body}, nil
}

func (handler *Handler) completeRequest(raw []byte) (any, error) {
	var completion requestCompletion
	if err := json.Unmarshal(raw, &completion); err != nil {
		return nil, fmt.Errorf("decode request completion: %w", err)
	}
	if strings.TrimSpace(completion.RequestID) == "" {
		return nil, fmt.Errorf("request completion ID is required")
	}
	handler.usage.completeRequest(completion.RequestID, strings.ToLower(strings.TrimSpace(completion.Outcome)))
	return struct{}{}, nil
}

func decodeRequestIntercept(raw []byte) (requestInterceptRequest, error) {
	var request requestInterceptRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return requestInterceptRequest{}, fmt.Errorf("decode request intercept: %w", err)
	}
	if strings.TrimSpace(request.RequestID) == "" {
		return requestInterceptRequest{}, fmt.Errorf("request intercept ID is required")
	}
	return request, nil
}

func metadataString(metadata map[string]json.RawMessage, key string) string {
	raw := metadata[key]
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
