package plugin

import (
	"encoding/json"
	"sort"
	"strings"
)

type optionalIdentity string

func (identity *optionalIdentity) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		*identity = ""
		return nil
	}
	*identity = optionalIdentity(strings.TrimSpace(value))
	return nil
}

func (identity optionalIdentity) Valid() bool {
	return identity != ""
}

func (identity optionalIdentity) String() string {
	return string(identity)
}

type executorMetadata struct {
	ExecutionSessionID optionalIdentity `json:"execution_session_id"`
	DerivedSessionID   optionalIdentity `json:"derived_session_id"`
	Fields             map[string]json.RawMessage
}

func (metadata *executorMetadata) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || string(raw) == "null" {
		*metadata = executorMetadata{}
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	*metadata = executorMetadata{Fields: fields}
	if value, ok := fields["execution_session_id"]; ok {
		if err := json.Unmarshal(value, &metadata.ExecutionSessionID); err != nil {
			return err
		}
	}
	if value, ok := fields["derived_session_id"]; ok {
		if err := json.Unmarshal(value, &metadata.DerivedSessionID); err != nil {
			return err
		}
	}
	return nil
}

type sessionIdentity struct {
	value string
}

func (identity sessionIdentity) String() string {
	return identity.value
}

func (request executorRequest) stableSessionIdentity() (sessionIdentity, bool) {
	if request.Metadata.ExecutionSessionID.Valid() {
		return newSessionIdentity(request.Metadata.ExecutionSessionID.String())
	}
	for _, key := range []string{"canonical_session_id", "session_id", "conversation_id"} {
		var value optionalIdentity
		if json.Unmarshal(request.Metadata.Fields[key], &value) == nil && value.Valid() {
			return newSessionIdentity(value.String())
		}
	}
	for _, name := range []string{"X-Session-ID", "Session_id", "X-Client-Request-Id", "canonical_session_id", "session_id", "session-id", "x-conversation-id", "conversation-id", "conversation_id"} {
		if identity, ok := sessionIdentityFromHeader(request.Headers, name); ok {
			return identity, true
		}
	}
	for _, body := range [][]byte{request.OriginalRequest, request.Payload} {
		if identity, ok := sessionIdentityFromBody(body); ok {
			return identity, true
		}
	}
	if request.Metadata.DerivedSessionID.Valid() {
		return newSessionIdentity(request.Metadata.DerivedSessionID.String())
	}
	return sessionIdentity{}, false
}

func sessionIdentityFromHeader(headers map[string][]string, name string) (sessionIdentity, bool) {
	if identity, ok := firstSessionIdentity(headers[name]); ok {
		return identity, true
	}
	keys := make([]string, 0)
	for key := range headers {
		if key != name && strings.EqualFold(key, name) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if identity, ok := firstSessionIdentity(headers[key]); ok {
			return identity, true
		}
	}
	return sessionIdentity{}, false
}

func firstSessionIdentity(values []string) (sessionIdentity, bool) {
	for _, value := range values {
		if identity, ok := newSessionIdentity(value); ok {
			return identity, true
		}
	}
	return sessionIdentity{}, false
}

func sessionIdentityFromBody(raw []byte) (sessionIdentity, bool) {
	var body map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &body) != nil {
		return sessionIdentity{}, false
	}
	for _, key := range []string{"canonical_session_id", "conversation_id", "session_id"} {
		var value optionalIdentity
		if json.Unmarshal(body[key], &value) == nil && value.Valid() {
			return newSessionIdentity(value.String())
		}
	}
	return sessionIdentity{}, false
}

func newSessionIdentity(value string) (sessionIdentity, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sessionIdentity{}, false
	}
	return sessionIdentity{value: value}, true
}
