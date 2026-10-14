package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorauth"
)

// Cursor usage changes slowly; caching keeps dashboard refreshes from hammering the account.
const (
	quotaCacheTTL      = 5 * time.Minute
	quotaErrorCacheTTL = 2 * time.Minute
)

type quotaUsageProvider interface {
	CurrentPeriodUsage(context.Context, string) (cursorapi.PeriodUsage, error)
	CurrentPlanInfo(context.Context, string) (cursorapi.PlanInfo, error)
}

type quotaSnapshot struct {
	Response quotaResponse
	Err      string
}

type quotaFetchRequest struct {
	AuthIndex   string `json:"auth_index"`
	StorageJSON []byte `json:"storage_json"`
}

type quotaSubscription struct {
	Plan     string `json:"plan,omitempty"`
	TierName string `json:"tierName,omitempty"`
}

type quotaMetric struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit,omitempty"`
	Format   string  `json:"format,omitempty"`
	Currency string  `json:"currency,omitempty"`
}

type quotaBucket struct {
	Window            string  `json:"window,omitempty"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime,omitempty"`
	Description       string  `json:"description,omitempty"`
}

type quotaGroup struct {
	DisplayName string        `json:"displayName,omitempty"`
	Buckets     []quotaBucket `json:"buckets,omitempty"`
}

type quotaResponse struct {
	Subscription *quotaSubscription `json:"subscription,omitempty"`
	Summary      []quotaMetric      `json:"summary,omitempty"`
	Groups       []quotaGroup       `json:"groups,omitempty"`
}

func quotaDescription() map[string]any {
	return map[string]any{
		"supported_providers": []string{"cursor"},
		"display_name":        "Cursor",
		"supports_reset":      false,
	}
}

func (handler *Handler) fetchQuota(ctx context.Context, raw []byte) (any, error) {
	var request quotaFetchRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode quota request: %w", err)
	}
	credential, err := cursorauth.ParseCredentials(request.StorageJSON)
	if err != nil {
		return nil, err
	}
	snapshot := handler.quotaFor(ctx, credential.AccessToken)
	if snapshot.Err != "" {
		return nil, errors.New(snapshot.Err)
	}
	return snapshot.Response, nil
}

// quotaFor returns cached usage, querying Cursor at most once per TTL per token.
func (handler *Handler) quotaFor(ctx context.Context, accessToken string) quotaSnapshot {
	provider, ok := handler.cursor.(quotaUsageProvider)
	if !ok {
		return quotaSnapshot{Err: "Cursor usage API is unavailable in this client"}
	}
	key := tokenFingerprint(accessToken)
	if cached, found := handler.quotas.get(key, time.Now()); found {
		return cached
	}
	usage, err := provider.CurrentPeriodUsage(ctx, accessToken)
	if err != nil {
		if ctx.Err() != nil {
			return quotaSnapshot{Err: ctx.Err().Error()}
		}
		snapshot := quotaSnapshot{Err: "query Cursor usage: " + err.Error()}
		handler.quotas.put(key, snapshot, quotaErrorCacheTTL, time.Now())
		return snapshot
	}
	plan, planErr := provider.CurrentPlanInfo(ctx, accessToken)
	if planErr != nil {
		plan = cursorapi.PlanInfo{}
	}
	snapshot := quotaSnapshot{Response: buildQuotaResponse(usage, plan)}
	handler.quotas.put(key, snapshot, quotaCacheTTL, time.Now())
	return snapshot
}

func buildQuotaResponse(usage cursorapi.PeriodUsage, plan cursorapi.PlanInfo) quotaResponse {
	response := quotaResponse{Summary: []quotaMetric{}, Groups: []quotaGroup{}}
	if plan.PlanName != "" {
		response.Subscription = &quotaSubscription{Plan: plan.PlanName, TierName: plan.PlanName}
	}
	resetTime := ""
	if end := usage.BillingCycleEnd; !end.IsZero() {
		resetTime = end.Format(time.RFC3339)
	} else if !plan.BillingCycleEnd.IsZero() {
		resetTime = plan.BillingCycleEnd.Format(time.RFC3339)
	}
	// Cursor's dashboard shows only the two percentage pools; the raw cent values in
	// planUsage are internal accounting and do not match what the user sees.
	if usage.Plan != nil {
		planUsage := usage.Plan
		buckets := make([]quotaBucket, 0, 2)
		if planUsage.AutoPercentUsed != nil {
			buckets = append(buckets, percentBucket("Cursor Models (Grok + Composer)", *planUsage.AutoPercentUsed, resetTime))
		}
		if planUsage.APIPercentUsed != nil {
			buckets = append(buckets, percentBucket("Other Models", *planUsage.APIPercentUsed, resetTime))
		}
		if len(buckets) == 0 && planUsage.TotalPercentUsed != nil {
			buckets = append(buckets, percentBucket("Included usage", *planUsage.TotalPercentUsed, resetTime))
		}
		if len(buckets) > 0 {
			response.Groups = append(response.Groups, quotaGroup{DisplayName: "Included in " + planLabel(plan), Buckets: buckets})
		}
	}
	if spend := usage.SpendLimit; spend != nil && spend.IndividualLimit > 0 {
		response.Summary = append(response.Summary,
			quotaMetric{Key: "on_demand_spend", Label: "On-demand", Value: cents(spend.IndividualUsed), Format: "currency", Currency: "USD"},
			quotaMetric{Key: "on_demand_limit", Label: "Monthly limit", Value: cents(spend.IndividualLimit), Format: "currency", Currency: "USD"},
		)
		response.Groups = append(response.Groups, quotaGroup{DisplayName: "On-Demand Usage", Buckets: []quotaBucket{{
			Window:            "monthly",
			RemainingFraction: clampFraction(1 - spend.IndividualUsed/spend.IndividualLimit),
			ResetTime:         resetTime,
			Description:       "On-Demand Spending",
		}}})
	}
	return response
}

func planLabel(plan cursorapi.PlanInfo) string {
	if plan.PlanName == "" {
		return "plan"
	}
	return plan.PlanName
}

// percentBucket carries its label in description: the CLIProxyAPI host keeps only
// window, remainingFraction, resetTime and description for plugin buckets.
func percentBucket(label string, percentUsed float64, resetTime string) quotaBucket {
	return quotaBucket{Window: "monthly", RemainingFraction: clampFraction(1 - percentUsed/100), ResetTime: resetTime, Description: label}
}

func cents(value float64) float64 {
	return math.Round(value) / 100
}

func clampFraction(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// managementQuotaStatus summarizes cached quota for the plugin status page.
func (handler *Handler) managementQuotaStatus(ctx context.Context, accessToken string) cursorQuotaStatus {
	snapshot := handler.quotaFor(ctx, accessToken)
	if snapshot.Err != "" {
		return cursorQuotaStatus{Status: "unavailable", Reason: snapshot.Err}
	}
	status := cursorQuotaStatus{Status: "available"}
	if snapshot.Response.Subscription != nil {
		status.Plan = snapshot.Response.Subscription.Plan
	}
	parts := make([]string, 0, len(snapshot.Response.Summary)+2)
	for _, group := range snapshot.Response.Groups {
		for _, bucket := range group.Buckets {
			parts = append(parts, fmt.Sprintf("%s %.0f%% used", bucket.Description, math.Round((1-bucket.RemainingFraction)*100)))
		}
	}
	for _, metric := range snapshot.Response.Summary {
		parts = append(parts, fmt.Sprintf("%s $%.2f", metric.Label, metric.Value))
	}
	status.Reason = strings.Join(parts, " · ")
	status.Quota = &snapshot.Response
	return status
}
