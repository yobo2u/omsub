package cursorapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidArgument = errors.New("Cursor invalid argument")

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
