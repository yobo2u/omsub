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

type modelContextProvider interface {
	ModelContextLengths(context.Context, string) (map[string]int64, error)
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
	catalog, err := handler.modelCatalog(ctx, credentials.AccessToken, true)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(catalog.Models))
	for _, model := range catalog.Models {
		names[model.ID] = model.DisplayName
	}
	return modelResponseWithNames(filterDisabledModels(catalogIDs(catalog.Models), credentials.DisabledModels), catalog.ContextLengths, names), nil
}

func filterDisabledModels(models, disabled []string) []string {
	if len(disabled) == 0 {
		return append([]string(nil), models...)
	}
	blocked := make(map[string]struct{}, len(disabled))
	for _, id := range disabled {
		normalized := strings.TrimPrefix(strings.TrimSpace(id), "cursor/")
		if normalized != "" {
			blocked[normalized] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(models))
	for _, id := range models {
		normalized := strings.TrimPrefix(strings.TrimSpace(id), "cursor/")
		if _, found := blocked[normalized]; !found && normalized != "" {
			filtered = append(filtered, normalized)
		}
	}
	return filtered
}

func modelResponse(ids []string, contexts map[string]int64) any {
	return modelResponseWithNames(ids, contexts, nil)
}

func modelResponseWithNames(ids []string, contexts map[string]int64, names map[string]string) any {
	models := make([]modelInfo, 0, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		displayName := "Cursor " + id
		if name := strings.TrimSpace(names[id]); name != "" {
			displayName = "Cursor " + name
		}
		models = append(models, modelInfo{
			ID:                         "cursor/" + id,
			Object:                     "model",
			OwnedBy:                    "cursor",
			DisplayName:                displayName,
			SupportedGenerationMethods: []string{"chat"},
			SupportedInputModalities:   []string{"text", "image"},
			SupportedOutputModalities:  []string{"text", "image"},
			ContextLength:              effectiveContextLength(contexts[id]),
			NativeContextLength:        contexts[id],
			ClientContextLimit:         clientContextLimit,
			UserDefined:                true,
		})
	}
	return struct {
		Provider string      `json:"Provider"`
		Models   []modelInfo `json:"Models"`
	}{Provider: "cursor", Models: models}
}
