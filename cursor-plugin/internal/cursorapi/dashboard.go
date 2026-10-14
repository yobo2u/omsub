package cursorapi

import (
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

const maxDashboardResponseBytes = 256 << 10

// PlanUsage is the included-usage section of DashboardService/GetCurrentPeriodUsage.
// Spend values are expressed in US cents.
type PlanUsage struct {
	TotalSpend       float64  `json:"totalSpend"`
	IncludedSpend    float64  `json:"includedSpend"`
	Remaining        float64  `json:"remaining"`
	Limit            float64  `json:"limit"`
	AutoPercentUsed  *float64 `json:"autoPercentUsed"`
	APIPercentUsed   *float64 `json:"apiPercentUsed"`
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
}

// SpendLimitUsage is the on-demand spend section of GetCurrentPeriodUsage.
type SpendLimitUsage struct {
	TotalSpend      float64 `json:"totalSpend"`
	IndividualLimit float64 `json:"individualLimit"`
	IndividualUsed  float64 `json:"individualUsed"`
	LimitType       string  `json:"limitType"`
}

// PeriodUsage is the normalized current billing period usage of a Cursor account.
type PeriodUsage struct {
	BillingCycleStart time.Time
	BillingCycleEnd   time.Time
	Plan              *PlanUsage
	SpendLimit        *SpendLimitUsage
	DisplayMessage    string
}

// PlanInfo describes the Cursor subscription plan.
type PlanInfo struct {
	PlanName        string
	Price           string
	BillingCycleEnd time.Time
}

// CurrentPeriodUsage queries DashboardService/GetCurrentPeriodUsage.
func (client *Client) CurrentPeriodUsage(ctx context.Context, accessToken string) (PeriodUsage, error) {
	var payload struct {
		BillingCycleStart json.RawMessage  `json:"billingCycleStart"`
		BillingCycleEnd   json.RawMessage  `json:"billingCycleEnd"`
		PlanUsage         *PlanUsage       `json:"planUsage"`
		SpendLimitUsage   *SpendLimitUsage `json:"spendLimitUsage"`
		DisplayMessage    string           `json:"displayMessage"`
	}
	if err := client.dashboardCall(ctx, accessToken, "GetCurrentPeriodUsage", &payload); err != nil {
		return PeriodUsage{}, err
	}
	return PeriodUsage{
		BillingCycleStart: parseMillis(payload.BillingCycleStart),
		BillingCycleEnd:   parseMillis(payload.BillingCycleEnd),
		Plan:              payload.PlanUsage,
		SpendLimit:        payload.SpendLimitUsage,
		DisplayMessage:    strings.TrimSpace(payload.DisplayMessage),
	}, nil
}

// CurrentPlanInfo queries DashboardService/GetPlanInfo.
func (client *Client) CurrentPlanInfo(ctx context.Context, accessToken string) (PlanInfo, error) {
	var payload struct {
		PlanInfo struct {
			PlanName        string          `json:"planName"`
			Price           string          `json:"price"`
			BillingCycleEnd json.RawMessage `json:"billingCycleEnd"`
		} `json:"planInfo"`
	}
	if err := client.dashboardCall(ctx, accessToken, "GetPlanInfo", &payload); err != nil {
		return PlanInfo{}, err
	}
	return PlanInfo{
		PlanName:        strings.TrimSpace(payload.PlanInfo.PlanName),
		Price:           strings.TrimSpace(payload.PlanInfo.Price),
		BillingCycleEnd: parseMillis(payload.PlanInfo.BillingCycleEnd),
	}, nil
}

func (client *Client) dashboardCall(ctx context.Context, accessToken, method string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint("/aiserver.v1.DashboardService/"+method), strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("create Cursor %s request: %w", method, err)
	}
	client.applyHeaders(request, requestHeaders{accessToken: accessToken, sessionID: randomUUID()}, "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Cursor %s: %w", method, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxDashboardResponseBytes+1))
	if err := errors.Join(readErr, response.Body.Close()); err != nil {
		return fmt.Errorf("read Cursor %s: %w", method, err)
	}
	if response.StatusCode != http.StatusOK {
		return &DashboardError{Method: method, StatusCode: response.StatusCode}
	}
	if len(raw) > maxDashboardResponseBytes {
		return fmt.Errorf("Cursor %s response exceeds limit", method)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode Cursor %s: %w", method, err)
	}
	return nil
}

// DashboardError reports a non-200 DashboardService response without upstream body details.
type DashboardError struct {
	Method     string
	StatusCode int
}

func (err *DashboardError) Error() string {
	return fmt.Sprintf("Cursor %s returned HTTP %d", err.Method, err.StatusCode)
}

// parseMillis accepts Connect JSON int64 values encoded as strings or numbers.
func parseMillis(raw json.RawMessage) time.Time {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return time.Time{}
	}
	millis, err := strconv.ParseInt(text, 10, 64)
	if err != nil || millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}
