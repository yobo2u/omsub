package cursorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const dashboardTimeout = 10 * time.Second

type PeriodUsage struct {
	PlanName            string
	Price               string
	IncludedAmountCents int64
	DisplayMessage      string
	AutoDisplayMessage  string
	APIDisplayMessage   string
	IncludedPercentUsed float64
	AutoPercentUsed     float64
	APIPercentUsed      float64
	IncludedSpendCents  int64
	IncludedLimitCents  int64
	HasIncludedSpend    bool
	BillingCycleStart   time.Time
	BillingCycleEnd     time.Time
	GrokBotLabel        string
	GrokBotPercentUsed  float64
	HasGrokBotPercent   bool
	GrokBotPeriodStart  time.Time
	GrokBotResetsAt     time.Time
	OnDemandKind        string
	OnDemandUsedCents   int64
	OnDemandLimitCents  int64
	HasOnDemandLimit    bool
}

func (client *Client) CurrentPeriodUsage(ctx context.Context, accessToken string) (PeriodUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, dashboardTimeout)
	defer cancel()
	var period currentPeriodUsageResponse
	if err := client.postDashboard(ctx, accessToken, "GetCurrentPeriodUsage", &period); err != nil {
		return PeriodUsage{}, err
	}
	if period.PlanUsage == nil {
		return PeriodUsage{}, errors.New("Cursor period usage did not include plan usage")
	}
	var plan planInfoResponse
	_ = client.postDashboard(ctx, accessToken, "GetPlanInfo", &plan)
	var limit hardLimitResponse
	_ = client.postDashboard(ctx, accessToken, "GetHardLimit", &limit)
	usage := assemblePeriodUsage(period, plan, limit)
	var sand sandUsageResponse
	if err := client.postDashboard(ctx, accessToken, "GetSandUsageStatus", &sand); err == nil {
		applySandUsage(&usage, sand)
	}
	return usage, nil
}

func (client *Client) postDashboard(ctx context.Context, accessToken string, method string, response any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint("/aiserver.v1.DashboardService/"+method), bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("create Cursor %s request: %w", method, err)
	}
	client.applyHeaders(request, requestHeaders{accessToken: accessToken, sessionID: randomUUID()}, "application/json")
	httpResponse, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Cursor %s: %w", method, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxModelResponseBytes+1))
	closeErr := httpResponse.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return fmt.Errorf("read Cursor %s: %w", method, err)
	}
	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("Cursor %s returned HTTP %d", method, httpResponse.StatusCode)
	}
	if len(raw) > maxModelResponseBytes {
		return fmt.Errorf("Cursor %s response exceeds 4 MiB", method)
	}
	if err := json.Unmarshal(raw, response); err != nil {
		return fmt.Errorf("decode Cursor %s: %w", method, err)
	}
	return nil
}

type currentPeriodUsageResponse struct {
	BillingCycleStart                flexInt64   `json:"billingCycleStart"`
	BillingCycleEnd                  flexInt64   `json:"billingCycleEnd"`
	PlanUsage                        *planUsage  `json:"planUsage"`
	SpendLimitUsage                  *spendLimit `json:"spendLimitUsage"`
	DisplayMessage                   string      `json:"displayMessage"`
	AutoModelSelectedDisplayMessage  string      `json:"autoModelSelectedDisplayMessage"`
	NamedModelSelectedDisplayMessage string      `json:"namedModelSelectedDisplayMessage"`
}

type planUsage struct {
	IncludedSpend    flexInt64   `json:"includedSpend"`
	Limit            flexInt64   `json:"limit"`
	AutoPercentUsed  flexFloat64 `json:"autoPercentUsed"`
	APIPercentUsed   flexFloat64 `json:"apiPercentUsed"`
	TotalPercentUsed flexFloat64 `json:"totalPercentUsed"`
}

type spendLimit struct {
	IndividualLimit flexInt64 `json:"individualLimit"`
	IndividualUsed  flexInt64 `json:"individualUsed"`
	LimitType       string    `json:"limitType"`
}

type planInfoResponse struct {
	PlanInfo *struct {
		PlanName            string    `json:"planName"`
		IncludedAmountCents flexInt64 `json:"includedAmountCents"`
		Price               string    `json:"price"`
		BillingCycleEnd     flexInt64 `json:"billingCycleEnd"`
	} `json:"planInfo"`
}

type hardLimitResponse struct {
	HardLimit           flexInt64 `json:"hardLimit"`
	NoUsageBasedAllowed bool      `json:"noUsageBasedAllowed"`
}

type sandUsageResponse struct {
	CurrentPeriodStart    string      `json:"currentPeriodStart"`
	NextResetTimestampUtc string      `json:"nextResetTimestampUtc"`
	UsagePercent          flexFloat64 `json:"usagePercent"`
	GrokPlanLabel         string      `json:"grokPlanLabel"`
}

func assemblePeriodUsage(period currentPeriodUsageResponse, plan planInfoResponse, limit hardLimitResponse) PeriodUsage {
	usage := PeriodUsage{OnDemandKind: "unavailable"}
	planUsage := period.PlanUsage
	if planUsage.TotalPercentUsed.set {
		usage.IncludedPercentUsed = planUsage.TotalPercentUsed.value
	} else if planUsage.Limit.value > 0 {
		usage.IncludedPercentUsed = float64(planUsage.IncludedSpend.value) / float64(planUsage.Limit.value) * 100
	}
	usage.AutoPercentUsed = planUsage.AutoPercentUsed.value
	usage.APIPercentUsed = planUsage.APIPercentUsed.value
	if planUsage.IncludedSpend.set || planUsage.Limit.set {
		usage.IncludedSpendCents = planUsage.IncludedSpend.value
		usage.IncludedLimitCents = planUsage.Limit.value
		usage.HasIncludedSpend = true
	}
	usage.DisplayMessage = period.DisplayMessage
	usage.AutoDisplayMessage = period.AutoModelSelectedDisplayMessage
	usage.APIDisplayMessage = period.NamedModelSelectedDisplayMessage
	startMillis := int64(period.BillingCycleStart.value)
	endMillis := int64(period.BillingCycleEnd.value)
	if plan.PlanInfo != nil {
		usage.PlanName = plan.PlanInfo.PlanName
		usage.Price = plan.PlanInfo.Price
		usage.IncludedAmountCents = int64(plan.PlanInfo.IncludedAmountCents.value)
		if endMillis <= 0 {
			endMillis = int64(plan.PlanInfo.BillingCycleEnd.value)
		}
	}
	if startMillis > 0 {
		usage.BillingCycleStart = time.UnixMilli(startMillis).UTC()
	}
	if endMillis > 0 {
		usage.BillingCycleEnd = time.UnixMilli(endMillis).UTC()
	}
	if period.SpendLimitUsage != nil {
		usage.OnDemandUsedCents = int64(period.SpendLimitUsage.IndividualUsed.value)
		if period.SpendLimitUsage.IndividualLimit.set {
			usage.OnDemandLimitCents = int64(period.SpendLimitUsage.IndividualLimit.value)
			usage.HasOnDemandLimit = true
		}
	}
	switch {
	case limit.NoUsageBasedAllowed || (limit.HardLimit.set && limit.HardLimit.value <= 0):
		usage.OnDemandKind = "disabled"
	case limit.HardLimit.value >= 2147483647:
		usage.OnDemandKind = "unlimited"
	case limit.HardLimit.value > 0:
		usage.OnDemandKind = "fixed"
		usage.OnDemandLimitCents = int64(limit.HardLimit.value) * 100
		usage.HasOnDemandLimit = true
	case usage.HasOnDemandLimit:
		usage.OnDemandKind = "fixed"
	}
	return usage
}

func applySandUsage(usage *PeriodUsage, sand sandUsageResponse) {
	if sand.UsagePercent.set {
		usage.GrokBotPercentUsed = sand.UsagePercent.value
		usage.HasGrokBotPercent = true
	}
	usage.GrokBotLabel = sand.GrokPlanLabel
	if start, ok := parseDashboardTime(sand.CurrentPeriodStart); ok {
		usage.GrokBotPeriodStart = start
	}
	if end, ok := parseDashboardTime(sand.NextResetTimestampUtc); ok {
		usage.GrokBotResetsAt = end
	}
}

func parseDashboardTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

type flexInt64 struct {
	value int64
	set   bool
}

func (number *flexInt64) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		if text == "" {
			return nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		number.value = parsed
		number.set = true
		return nil
	}
	parsed, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return err
	}
	number.value = parsed
	number.set = true
	return nil
}

type flexFloat64 struct {
	value float64
	set   bool
}

func (number *flexFloat64) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		if text == "" {
			return nil
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return err
		}
		number.value = parsed
		number.set = true
		return nil
	}
	parsed, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return err
	}
	number.value = parsed
	number.set = true
	return nil
}
