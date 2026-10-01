package plugin

import (
	"encoding/json"
	"time"
)

type hostAuthFile struct {
	ID            string `json:"id"`
	AuthIndex     string `json:"auth_index"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Source        string `json:"source"`
	Type          string `json:"type"`
	Provider      string `json:"provider"`
	Label         string `json:"label"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Success       int64  `json:"success"`
	Failed        int64  `json:"failed"`
	RuntimeOnly   bool   `json:"runtime_only"`
}

type hostRuntimeStatus struct {
	Scope           string `json:"scope"`
	Status          string `json:"status"`
	StatusMessage   string `json:"status_message,omitempty"`
	SuccessAttempts int64  `json:"success_attempts"`
	FailedAttempts  int64  `json:"failed_attempts"`
}

type hostAuthListResponse struct {
	Files []hostAuthFile `json:"files"`
}

type hostAuthGetResponse struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name"`
	JSON      json.RawMessage `json:"json"`
}

type cursorQuotaStatus struct {
	Status              string   `json:"status"`
	Reason              string   `json:"reason,omitempty"`
	PlanName            string   `json:"plan_name,omitempty"`
	Price               string   `json:"price,omitempty"`
	DisplayMessage      string   `json:"display_message,omitempty"`
	AutoDisplayMessage  string   `json:"auto_display_message,omitempty"`
	APIDisplayMessage   string   `json:"api_display_message,omitempty"`
	IncludedPercentUsed *float64 `json:"included_percent_used,omitempty"`
	AutoPercentUsed     *float64 `json:"auto_percent_used,omitempty"`
	APIPercentUsed      *float64 `json:"api_percent_used,omitempty"`
	IncludedSpendCents  *int64   `json:"included_spend_cents,omitempty"`
	IncludedLimitCents  *int64   `json:"included_limit_cents,omitempty"`
	CycleStartAt        string   `json:"cycle_start_at,omitempty"`
	ResetsAt            string   `json:"resets_at,omitempty"`
	GrokBotLabel        string   `json:"grok_bot_label,omitempty"`
	GrokBotPercentUsed  *float64 `json:"grok_bot_percent_used,omitempty"`
	GrokBotCycleStartAt string   `json:"grok_bot_cycle_start_at,omitempty"`
	GrokBotResetsAt     string   `json:"grok_bot_resets_at,omitempty"`
	OnDemandKind        string   `json:"on_demand_kind,omitempty"`
	OnDemandUsedCents   *int64   `json:"on_demand_used_cents,omitempty"`
	OnDemandLimitCents  *int64   `json:"on_demand_limit_cents,omitempty"`
}

type cursorAccountStatus struct {
	AuthIndex          string                 `json:"auth_index"`
	Name               string                 `json:"name"`
	Label              string                 `json:"label"`
	Status             string                 `json:"status"`
	HostRuntime        hostRuntimeStatus      `json:"host_runtime"`
	SubscriptionQuota  cursorQuotaStatus      `json:"subscription_quota"`
	LocalUsage         localUsageStatus       `json:"local_usage"`
	CheckpointMetrics  checkpointMetricStatus `json:"checkpoint_metrics"`
	Models             []cursorModelStatus    `json:"models"`
	ModelContextStatus string                 `json:"model_context_status"`
}

type cursorManagementStatus struct {
	Provider          string                 `json:"provider"`
	GeneratedAt       time.Time              `json:"generated_at"`
	Accounts          []cursorAccountStatus  `json:"accounts"`
	CheckpointMetrics checkpointMetricStatus `json:"checkpoint_metrics"`
	ContextPolicy     contextPolicyStatus    `json:"context_policy"`
}

type disabledModelsUpdate struct {
	AuthIndex      string   `json:"auth_index"`
	DisabledModels []string `json:"disabled_models"`
}
