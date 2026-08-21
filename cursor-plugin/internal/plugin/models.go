package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cursorplugin/internal/cursorauth"
)

type authModelRequest struct {
	StorageJSON []byte `json:"StorageJSON"`
}

func (handler *Handler) modelsForAuth(ctx context.Context, raw []byte) (any, error) {
	var request authModelRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth model request: %w", err)
	}
	credentials, err := cursorauth.ParseCredentials(request.StorageJSON)
	if err != nil {
		return nil, err
	}
	models, err := handler.cursor.DiscoverModels(ctx, credentials.AccessToken)
	if err != nil {
		return nil, err
	}
	return modelResponse(models), nil
}

func modelResponse(ids []string) any {
	models := make([]modelInfo, 0, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		models = append(models, modelInfo{
			ID:                         "cursor/" + id,
			Object:                     "model",
			OwnedBy:                    "cursor",
			DisplayName:                "Cursor " + id,
			SupportedGenerationMethods: []string{"chat"},
			ContextLength:              200000,
			MaxCompletionTokens:        32768,
			UserDefined:                true,
		})
	}
	return struct {
		Provider string      `json:"Provider"`
		Models   []modelInfo `json:"Models"`
	}{Provider: "cursor", Models: models}
}
