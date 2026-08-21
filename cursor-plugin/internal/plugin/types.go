package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorauth"
	"cursorplugin/internal/cursorproto"
)

type CursorClient interface {
	Run(context.Context, cursorapi.RunInput, func(cursorproto.ServerEvent) error) error
	DiscoverModels(context.Context, string) ([]string, error)
}

type StreamEmitter interface {
	Emit(context.Context, string, []byte) error
	Close(string, error) error
}

type HostCaller interface {
	Call(context.Context, string, any) (json.RawMessage, error)
}

type Dependencies struct {
	Auth    *cursorauth.Service
	Cursor  CursorClient
	Emitter StreamEmitter
	Host    HostCaller
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type authData struct {
	Provider         string            `json:"Provider"`
	ID               string            `json:"ID"`
	FileName         string            `json:"FileName"`
	Label            string            `json:"Label"`
	StorageJSON      []byte            `json:"StorageJSON"`
	Metadata         map[string]string `json:"Metadata,omitempty"`
	Attributes       map[string]string `json:"Attributes,omitempty"`
	NextRefreshAfter time.Time         `json:"NextRefreshAfter,omitempty"`
}

type modelInfo struct {
	ID                         string   `json:"ID"`
	Object                     string   `json:"Object"`
	OwnedBy                    string   `json:"OwnedBy"`
	DisplayName                string   `json:"DisplayName"`
	SupportedGenerationMethods []string `json:"SupportedGenerationMethods"`
	ContextLength              int64    `json:"ContextLength"`
	MaxCompletionTokens        int64    `json:"MaxCompletionTokens"`
	UserDefined                bool     `json:"UserDefined"`
}

type executorRequest struct {
	Model           string          `json:"Model"`
	Format          string          `json:"Format"`
	OriginalRequest []byte          `json:"OriginalRequest"`
	Payload         []byte          `json:"Payload"`
	StorageJSON     []byte          `json:"StorageJSON"`
	StreamID        string          `json:"stream_id"`
	Metadata        json.RawMessage `json:"Metadata"`
}

type executorResponse struct {
	Payload []byte      `json:"Payload"`
	Headers http.Header `json:"Headers,omitempty"`
}
