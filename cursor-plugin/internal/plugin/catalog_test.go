package plugin

import (
	"context"
	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorproto"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type catalogTestClient struct {
	fakeModelCursorClient
	editorErr error
	calls     *int
}

func (c catalogTestClient) DiscoverModelDetails(context.Context, string) ([]cursorproto.ModelDetail, error) {
	return []cursorproto.ModelDetail{{ID: "cli-model", DisplayName: "CLI  Model\u200b"}}, nil
}
func (c catalogTestClient) EditorCatalog(context.Context, string) (cursorapi.EditorCatalog, error) {
	*c.calls++
	return cursorapi.EditorCatalog{Models: []cursorapi.EditorModel{
		{ID: "cli-model", Slugs: []string{"cli-model", "cli-model-high"}, SupportsAgent: true},
		{ID: "editor-model", DisplayName: "Editor Model", Slugs: []string{"editor-model", "editor-model-high"}, SupportsAgent: true},
		{ID: "chat-only", DisplayName: "Chat only", SupportsAgent: false},
	}, ContextLengths: map[string]int64{"editor-model": 200000}}, c.editorErr
}
func TestCatalogMergeAndCache(t *testing.T) {
	calls := 0
	h := NewHandler(Dependencies{Cursor: catalogTestClient{calls: &calls}})
	for range 3 {
		catalog, err := h.modelCatalog(context.Background(), "synthetic", false)
		require.NoError(t, err)
		require.Equal(t, []string{"cli-model", "editor-model", "editor-model-high"}, catalogIDs(catalog.Models))
		require.Equal(t, "CLI Model", catalog.Models[0].DisplayName)
		require.Equal(t, "Editor Model (high)", catalog.Models[2].DisplayName)
		require.True(t, catalog.Models[1].EditorOnly)
		require.Equal(t, int64(200000), catalog.ContextLengths["editor-model"])
	}
	require.Equal(t, 1, calls)
	_, err := h.modelCatalog(context.Background(), "synthetic", true)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}
func TestCatalogEditorFailurePreservesCLI(t *testing.T) {
	calls := 0
	h := NewHandler(Dependencies{Cursor: catalogTestClient{calls: &calls, editorErr: errors.New("unavailable")}})
	catalog, err := h.modelCatalog(context.Background(), "synthetic", false)
	require.NoError(t, err)
	require.Equal(t, []string{"cli-model"}, catalogIDs(catalog.Models))
	require.Equal(t, "unavailable", catalog.MetadataStatus)
}
func TestCatalogDisplayNames(t *testing.T) {
	require.Equal(t, "Grok High Fast", cleanDisplayName("Grok  High Fast\u200b\u200b"))
	require.Empty(t, cleanDisplayName(" \u200b "))
}
