package cursorapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type contextModel struct {
	Name               string   `json:"name"`
	ServerModelName    string   `json:"serverModelName"`
	ClientDisplayName  string   `json:"clientDisplayName"`
	LegacySlugs        []string `json:"legacySlugs"`
	IDAliases          []string `json:"idAliases"`
	ContextTokenLimit  int32    `json:"contextTokenLimit"`
	SupportsNonMaxMode *bool    `json:"supportsNonMaxMode"`
	SupportsAgent      *bool    `json:"supportsAgent"`
	Variants           []struct {
		LegacySlug string `json:"legacySlug"`
		IsMaxMode  bool   `json:"isMaxMode"`
	} `json:"variants"`
}

// EditorModel describes one model family from the Cursor editor catalog.
type EditorModel struct {
	// ID is the family identifier used by the editor (for example "glm-5p3").
	ID string
	// DisplayName is the editor label (for example "GLM 5.3").
	DisplayName string
	// Slugs lists the non-MAX variant identifiers that belong to the family.
	Slugs []string
	// ContextTokenLimit is the non-MAX context window, or zero when unknown.
	ContextTokenLimit int64
	// SupportsAgent reports whether the editor allows the family in agent mode.
	SupportsAgent bool
}

// ModelContextLengths describes the non-MAX execution mode used by this client.
func (client *Client) ModelContextLengths(ctx context.Context, accessToken string) (map[string]int64, error) {
	catalog, err := client.EditorCatalog(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return catalog.ContextLengths, nil
}

// EditorCatalog combines editor families and non-MAX context limits from one upstream call.
type EditorCatalog struct {
	Models         []EditorModel
	ContextLengths map[string]int64
}

// EditorCatalog fetches AvailableModels once and returns families plus context limits.
func (client *Client) EditorCatalog(ctx context.Context, accessToken string) (EditorCatalog, error) {
	models, err := client.editorCatalog(ctx, accessToken)
	if err != nil {
		return EditorCatalog{}, err
	}
	limits := make(map[string]int64)
	for _, model := range models {
		if model.ContextTokenLimit <= 0 || (model.SupportsNonMaxMode != nil && !*model.SupportsNonMaxMode) {
			continue
		}
		for _, id := range nonMaxIdentifiers(model) {
			limits[id] = int64(model.ContextTokenLimit)
		}
	}
	return EditorCatalog{Models: editorModels(models), ContextLengths: limits}, nil
}

func editorModels(models []contextModel) []EditorModel {
	out := make([]EditorModel, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.Name)
		if id == "" || (model.SupportsNonMaxMode != nil && !*model.SupportsNonMaxMode) {
			continue
		}
		item := EditorModel{
			ID:            id,
			DisplayName:   strings.TrimSpace(model.ClientDisplayName),
			SupportsAgent: model.SupportsAgent == nil || *model.SupportsAgent,
		}
		if model.ContextTokenLimit > 0 {
			item.ContextTokenLimit = int64(model.ContextTokenLimit)
		}
		seen := map[string]struct{}{}
		for _, slug := range append(append([]string(nil), model.LegacySlugs...), variantSlugs(model)...) {
			slug = strings.TrimSpace(slug)
			if slug == "" {
				continue
			}
			if _, exists := seen[slug]; exists {
				continue
			}
			seen[slug] = struct{}{}
			item.Slugs = append(item.Slugs, slug)
		}
		out = append(out, item)
	}
	return out
}

func (client *Client) editorCatalog(ctx context.Context, accessToken string) ([]contextModel, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body := strings.NewReader(`{"useModelParameters":true,"useReactModelPicker":true,"excludeMaxNamedModels":true}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint("/aiserver.v1.AiService/AvailableModels"), body)
	if err != nil {
		return nil, fmt.Errorf("create Cursor context metadata request: %w", err)
	}
	client.applyHeaders(request, requestHeaders{accessToken: accessToken, sessionID: randomUUID()}, "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Cursor context metadata: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err := errors.Join(readErr, response.Body.Close()); err != nil {
		return nil, fmt.Errorf("read Cursor context metadata: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cursor context metadata returned HTTP %d", response.StatusCode)
	}
	if len(raw) > maxModelResponseBytes {
		return nil, errors.New("Cursor context metadata exceeds 4 MiB")
	}
	var catalog struct {
		Models []contextModel `json:"models"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("decode Cursor context metadata: %w", err)
	}
	return catalog.Models, nil
}

func nonMaxIdentifiers(model contextModel) []string {
	ids := append([]string{model.Name, model.ServerModelName}, model.LegacySlugs...)
	ids = append(ids, model.IDAliases...)
	ids = append(ids, variantSlugs(model)...)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func variantSlugs(model contextModel) []string {
	out := make([]string, 0, len(model.Variants))
	for _, variant := range model.Variants {
		if !variant.IsMaxMode && variant.LegacySlug != "" {
			out = append(out, variant.LegacySlug)
		}
	}
	return out
}
