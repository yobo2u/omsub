package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxToolCallIDBytes = 64

func (turn *Turn) reserveToolCallID(raw string) (string, error) {
	id, err := canonicalToolCallID(raw)
	if err != nil {
		return "", err
	}
	if turn.toolIDOwners == nil {
		turn.toolIDOwners = make(map[string]string)
	}
	if owner, exists := turn.toolIDOwners[id]; exists {
		if owner == raw {
			return "", errors.New("duplicate Cursor tool call id")
		}
		for salt := 0; ; salt++ {
			id = hashedToolCallID(raw, salt)
			if _, collision := turn.toolIDOwners[id]; !collision {
				break
			}
		}
	}
	turn.toolIDOwners[id] = raw
	return id, nil
}

func canonicalToolCallID(raw string) (string, error) {
	if validToolCallID(raw) {
		return raw, nil
	}
	parts := strings.FieldsFunc(raw, unicode.IsControl)
	for _, part := range parts {
		if validToolCallID(part) && (strings.HasPrefix(part, "call-") || strings.HasPrefix(part, "call_")) {
			return part, nil
		}
	}
	for _, part := range parts {
		if validToolCallID(part) {
			return part, nil
		}
	}
	if len(parts) == 0 {
		return "", errors.New("Cursor tool call id is empty or contains only control characters")
	}
	return hashedToolCallID(raw, 0), nil
}

func validToolCallID(id string) bool {
	if id == "" || len(id) > maxToolCallIDBytes || !utf8.ValidString(id) {
		return false
	}
	for _, character := range id {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func hashedToolCallID(raw string, salt int) string {
	if salt > 0 {
		raw = strconv.Itoa(salt) + "\x00" + raw
	}
	digest := sha256.Sum256([]byte(raw))
	return "call_cursor_" + hex.EncodeToString(digest[:26])
}
