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
	Code    string `json:"code"`
	Message string `json:"message"`
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
	code := strings.TrimSpace(response.Error.Code)
	message := strings.TrimSpace(response.Error.Message)
	if strings.EqualFold(code, "invalid_argument") {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, message)
	}
	if code == "" && message == "" {
		return errors.New("Cursor stream failed")
	}
	if code == "" {
		return fmt.Errorf("Cursor stream failed: %s", message)
	}
	if message == "" {
		return fmt.Errorf("Cursor stream failed: %s", code)
	}
	return fmt.Errorf("Cursor stream failed: %s: %s", code, message)
}
