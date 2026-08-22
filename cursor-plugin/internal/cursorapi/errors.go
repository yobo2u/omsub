package cursorapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidArgument = errors.New("Cursor invalid argument")

type connectEndStreamResponse struct {
	Error *connectWireError `json:"error"`
}

type connectWireError struct {
	Code string `json:"code"`
}

func IsInvalidArgument(err error) bool {
	return errors.Is(err, ErrInvalidArgument)
}

func runStatusError(status int, body []byte) error {
	var response struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &response) == nil && strings.EqualFold(response.Code, "invalid_argument") {
		return fmt.Errorf("%w: Cursor Run returned HTTP %d", ErrInvalidArgument, status)
	}
	return fmt.Errorf("Cursor Run returned HTTP %d", status)
}

func connectEndStreamError(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	var response connectEndStreamResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return fmt.Errorf("decode Cursor Connect end stream: %w", err)
	}
	if response.Error == nil {
		return nil
	}
	code := strings.ToLower(strings.TrimSpace(response.Error.Code))
	switch code {
	case "invalid_argument":
		return fmt.Errorf("%w: Cursor rejected the request", ErrInvalidArgument)
	case "canceled", "unknown", "deadline_exceeded", "not_found", "already_exists", "permission_denied",
		"resource_exhausted", "failed_precondition", "aborted", "out_of_range", "unimplemented", "internal",
		"unavailable", "data_loss", "unauthenticated":
		return fmt.Errorf("Cursor stream failed: %s", code)
	default:
		return errors.New("Cursor stream failed")
	}
}
