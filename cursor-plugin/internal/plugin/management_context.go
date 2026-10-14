package plugin

import (
	"cursorplugin/internal/cursorauth"
)

type cursorModelStatus struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"display_name,omitempty"`
	EditorOnly          bool   `json:"editor_only,omitempty"`
	Disabled            bool   `json:"disabled"`
	NativeContextLength int64  `json:"native_context_length,omitempty"`
	ClientContextLimit  int64  `json:"client_context_limit"`
	ContextLength       int64  `json:"context_length,omitempty"`
}

type contextPolicyStatus struct {
	ClientContextLimit int64  `json:"client_context_limit"`
	CountMethod        string `json:"count_method"`
	Reserve            int64  `json:"reserve_units"`
	NativeMode         string `json:"native_mode"`
}

func currentContextPolicy() contextPolicyStatus {
	return contextPolicyStatus{ClientContextLimit: clientContextLimit, CountMethod: contextCountMethod, Reserve: contextReserve, NativeMode: "non_max"}
}

func (handler *Handler) managementModelContexts(catalog modelCatalog, credential cursorauth.Credentials) []cursorModelStatus {
	contexts := catalog.ContextLengths
	disabled := normalizedModelSet(credential.DisabledModels)
	items := make([]cursorModelStatus, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		id := normalizeModelID(model.ID)
		if id == "" {
			continue
		}
		_, blocked := disabled[id]
		items = append(items, cursorModelStatus{
			ID: id, DisplayName: model.DisplayName, EditorOnly: model.EditorOnly, Disabled: blocked, NativeContextLength: max(contexts[id], 0),
			ClientContextLimit: clientContextLimit, ContextLength: effectiveContextLength(contexts[id]),
		})
	}
	return items
}
