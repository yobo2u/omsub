package plugin

import (
	"encoding/json"
	"sync"
	"time"
)

type usageDetail struct {
	InputTokens     int64 `json:"InputTokens"`
	OutputTokens    int64 `json:"OutputTokens"`
	ReasoningTokens int64 `json:"ReasoningTokens"`
	TotalTokens     int64 `json:"TotalTokens"`
}

type usageRecord struct {
	Provider  string      `json:"Provider"`
	AuthIndex string      `json:"AuthIndex"`
	Failed    bool        `json:"Failed"`
	Detail    usageDetail `json:"Detail"`
}

type localUsageStatus struct {
	Scope        string    `json:"scope"`
	StartedAt    time.Time `json:"started_at"`
	Estimated    bool      `json:"estimated"`
	Requests     int64     `json:"requests"`
	Failed       int64     `json:"failed"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
}

type usageStore struct {
	mu        sync.RWMutex
	startedAt time.Time
	byAuth    map[string]localUsageStatus
}

func newUsageStore() *usageStore {
	return &usageStore{startedAt: time.Now().UTC(), byAuth: make(map[string]localUsageStatus)}
}

func (handler *Handler) handleUsage(raw []byte) (any, error) {
	var record usageRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	if record.Provider != "cursor" || record.AuthIndex == "" {
		return struct{}{}, nil
	}
	handler.usage.add(record)
	return struct{}{}, nil
}

func (store *usageStore) add(record usageRecord) {
	store.mu.Lock()
	defer store.mu.Unlock()
	usage := store.byAuth[record.AuthIndex]
	usage.Scope = "plugin_process"
	usage.StartedAt = store.startedAt
	usage.Estimated = true
	usage.Requests++
	if record.Failed {
		usage.Failed++
	}
	usage.InputTokens += record.Detail.InputTokens
	usage.OutputTokens += record.Detail.OutputTokens
	total := record.Detail.TotalTokens
	if total == 0 {
		total = record.Detail.InputTokens + record.Detail.OutputTokens + record.Detail.ReasoningTokens
	}
	usage.TotalTokens += total
	store.byAuth[record.AuthIndex] = usage
}

func (store *usageStore) snapshot(authIndex string) localUsageStatus {
	store.mu.RLock()
	defer store.mu.RUnlock()
	usage := store.byAuth[authIndex]
	usage.Scope = "plugin_process"
	usage.StartedAt = store.startedAt
	usage.Estimated = true
	return usage
}
