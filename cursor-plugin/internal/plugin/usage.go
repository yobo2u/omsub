package plugin

import (
	"sync"
	"time"

	"cursorplugin/internal/openai"
)

type localUsageStatus struct {
	Scope        string     `json:"scope"`
	StartedAt    time.Time  `json:"started_at"`
	Estimated    bool       `json:"estimated"`
	ExecutorRuns int64      `json:"executor_runs"`
	Requests     int64      `json:"requests"`
	Succeeded    int64      `json:"succeeded"`
	Failed       int64      `json:"failed"`
	LastOutcome  string     `json:"last_outcome,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	TotalTokens  int64      `json:"total_tokens"`
}

type usageStore struct {
	mu          sync.RWMutex
	startedAt   time.Time
	byAuth      map[string]localUsageStatus
	requestAuth map[string]string
	checkpoints *checkpointMetrics
}

func newUsageStore() *usageStore {
	return &usageStore{
		startedAt: time.Now().UTC(), byAuth: make(map[string]localUsageStatus),
		requestAuth: make(map[string]string), checkpoints: newCheckpointMetrics(),
	}
}

func (store *usageStore) recordTokens(authID string, usageEstimate openai.Usage) {
	if authID == "" {
		return
	}
	now := time.Now().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()
	usage := store.byAuth[authID]
	usage.Scope = "plugin_process"
	usage.StartedAt = store.startedAt
	usage.Estimated = true
	usage.ExecutorRuns++
	usage.UpdatedAt = &now
	usage.InputTokens += int64(usageEstimate.PromptTokens)
	usage.OutputTokens += int64(usageEstimate.CompletionTokens)
	usage.TotalTokens += int64(usageEstimate.TotalTokens)
	store.byAuth[authID] = usage
}

func (store *usageStore) selectRequestAuth(requestID, authID string) {
	if requestID == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if authID == "" {
		delete(store.requestAuth, requestID)
		return
	}
	store.requestAuth[requestID] = authID
}

func (store *usageStore) completeRequest(requestID, authID string, selected bool, outcome string) {
	if requestID == "" {
		return
	}
	now := time.Now().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()
	trackedAuthID := store.requestAuth[requestID]
	delete(store.requestAuth, requestID)
	if !selected {
		authID = trackedAuthID
	}
	if authID == "" {
		return
	}
	usage := store.byAuth[authID]
	usage.Scope = "plugin_process"
	usage.StartedAt = store.startedAt
	usage.Estimated = true
	usage.Requests++
	usage.LastOutcome = outcome
	usage.UpdatedAt = &now
	if outcome == "succeeded" {
		usage.Succeeded++
	} else {
		usage.Failed++
	}
	store.byAuth[authID] = usage
}

func (store *usageStore) snapshot(authID string) localUsageStatus {
	store.mu.RLock()
	defer store.mu.RUnlock()
	usage := store.byAuth[authID]
	usage.Scope = "plugin_process"
	usage.StartedAt = store.startedAt
	usage.Estimated = true
	return usage
}

func mergeLocalUsage(left, right localUsageStatus) localUsageStatus {
	merged := left
	merged.Scope = "plugin_process"
	if merged.StartedAt.IsZero() || (!right.StartedAt.IsZero() && right.StartedAt.Before(merged.StartedAt)) {
		merged.StartedAt = right.StartedAt
	}
	merged.Estimated = left.Estimated || right.Estimated
	merged.ExecutorRuns += right.ExecutorRuns
	merged.Requests += right.Requests
	merged.Succeeded += right.Succeeded
	merged.Failed += right.Failed
	merged.InputTokens += right.InputTokens
	merged.OutputTokens += right.OutputTokens
	merged.TotalTokens += right.TotalTokens
	if right.UpdatedAt != nil && (merged.UpdatedAt == nil || !right.UpdatedAt.Before(*merged.UpdatedAt)) {
		updatedAt := *right.UpdatedAt
		merged.UpdatedAt = &updatedAt
		merged.LastOutcome = right.LastOutcome
	}
	return merged
}
