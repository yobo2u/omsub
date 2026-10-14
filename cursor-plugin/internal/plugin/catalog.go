package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
	"unicode"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorproto"
)

const catalogCacheTTL = 10 * time.Minute

type modelDetailProvider interface {
	DiscoverModelDetails(context.Context, string) ([]cursorproto.ModelDetail, error)
}

type editorCatalogProvider interface {
	EditorCatalog(context.Context, string) (cursorapi.EditorCatalog, error)
}

type catalogModel struct {
	ID          string
	DisplayName string
	EditorOnly  bool
}

type modelCatalog struct {
	Models         []catalogModel
	ContextLengths map[string]int64
	MetadataStatus string
}

// ttlCache keeps short-lived upstream responses keyed by a token fingerprint so that
// management page refreshes do not multiply requests to Cursor.
type ttlCache[T any] struct {
	mu      sync.Mutex
	entries map[string]ttlEntry[T]
}

type ttlEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func (cache *ttlCache[T]) get(key string, now time.Time) (T, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok || now.After(entry.expiresAt) {
		var zero T
		return zero, false
	}
	return entry.value, true
}

func (cache *ttlCache[T]) put(key string, value T, ttl time.Duration, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = make(map[string]ttlEntry[T])
	}
	for existing, entry := range cache.entries {
		if now.After(entry.expiresAt) {
			delete(cache.entries, existing)
		}
	}
	cache.entries[key] = ttlEntry[T]{value: value, expiresAt: now.Add(ttl)}
}

func tokenFingerprint(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return hex.EncodeToString(sum[:8])
}

// modelCatalog merges the CLI catalog (GetUsableModels) with editor-only families
// from AvailableModels, such as models the CLI endpoint does not advertise yet.
// Fresh lookups (host model registration) bypass the cache but refresh it; cached
// lookups serve the management page, which may be polled frequently.
func (handler *Handler) modelCatalog(ctx context.Context, accessToken string, fresh bool) (modelCatalog, error) {
	key := tokenFingerprint(accessToken)
	if !fresh {
		if cached, ok := handler.catalogs.get(key, time.Now()); ok {
			return cached, nil
		}
	}
	catalog, err := handler.fetchModelCatalog(ctx, accessToken)
	if err != nil {
		return modelCatalog{}, err
	}
	if catalog.MetadataStatus == "available" {
		handler.catalogs.put(key, catalog, catalogCacheTTL, time.Now())
	}
	return catalog, nil
}

func (handler *Handler) fetchModelCatalog(ctx context.Context, accessToken string) (modelCatalog, error) {
	var details []cursorproto.ModelDetail
	if provider, ok := handler.cursor.(modelDetailProvider); ok {
		var err error
		if details, err = provider.DiscoverModelDetails(ctx, accessToken); err != nil {
			return modelCatalog{}, err
		}
	} else {
		ids, err := handler.cursor.DiscoverModels(ctx, accessToken)
		if err != nil {
			return modelCatalog{}, err
		}
		for _, id := range ids {
			details = append(details, cursorproto.ModelDetail{ID: id})
		}
	}
	catalog := modelCatalog{MetadataStatus: "unsupported"}
	seen := make(map[string]struct{}, len(details))
	for _, detail := range details {
		id := normalizeModelID(detail.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		catalog.Models = append(catalog.Models, catalogModel{ID: id, DisplayName: cleanDisplayName(detail.DisplayName)})
	}
	switch provider := handler.cursor.(type) {
	case editorCatalogProvider:
		editor, err := provider.EditorCatalog(ctx, accessToken)
		if err != nil {
			if ctx.Err() != nil {
				return modelCatalog{}, ctx.Err()
			}
			catalog.MetadataStatus = "unavailable"
			break
		}
		catalog.MetadataStatus = "available"
		catalog.ContextLengths = editor.ContextLengths
		catalog.Models = append(catalog.Models, editorOnlyModels(editor.Models, seen)...)
	case modelContextProvider:
		contexts, err := provider.ModelContextLengths(ctx, accessToken)
		if err != nil {
			if ctx.Err() != nil {
				return modelCatalog{}, ctx.Err()
			}
			catalog.MetadataStatus = "unavailable"
			break
		}
		catalog.MetadataStatus = "available"
		catalog.ContextLengths = contexts
	}
	return catalog, nil
}

// editorOnlyModels returns agent-capable editor families whose identifiers are all
// missing from the CLI catalog. Families partially present in the CLI catalog are
// skipped because the CLI already publishes their runnable variants.
func editorOnlyModels(families []cursorapi.EditorModel, seen map[string]struct{}) []catalogModel {
	out := make([]catalogModel, 0)
	for _, family := range families {
		if !family.SupportsAgent {
			continue
		}
		ids := family.Slugs
		if len(ids) == 0 {
			ids = []string{family.ID}
		}
		present := false
		for _, id := range append([]string{family.ID}, ids...) {
			if _, exists := seen[normalizeModelID(id)]; exists {
				present = true
				break
			}
		}
		if present {
			continue
		}
		for _, rawID := range ids {
			id := normalizeModelID(rawID)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, catalogModel{ID: id, DisplayName: editorVariantName(family, id), EditorOnly: true})
		}
	}
	return out
}

// cleanDisplayName drops invisible format characters Cursor embeds in some names
// and collapses repeated whitespace.
func cleanDisplayName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	return strings.Join(strings.Fields(name), " ")
}

func editorVariantName(family cursorapi.EditorModel, id string) string {
	name := cleanDisplayName(family.DisplayName)
	if name == "" {
		return ""
	}
	suffix := strings.TrimPrefix(strings.TrimPrefix(id, family.ID), "-")
	if suffix == "" || suffix == id {
		return name
	}
	return name + " (" + suffix + ")"
}

func catalogIDs(models []catalogModel) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}
