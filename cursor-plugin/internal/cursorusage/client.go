package cursorusage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL          = "https://cursor.com"
	defaultTimeout          = 15 * time.Second
	maxUsageResponseBytes   = 1 << 20
	sessionCookieName       = "WorkosCursorSessionToken"
	cursorOrigin            = "https://cursor.com"
	cursorDashboardReferrer = "https://cursor.com/dashboard"
	individualTeamID        = -1
	cursorModelTier         = 2
	otherModelTier          = 1
)

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type Snapshot struct {
	MembershipType    string
	BillingCycleStart time.Time
	BillingCycleEnd   time.Time
	DisplayMessage    string
	AutoModelMessage  string
	NamedModelMessage string
	CursorModels      Bucket
	OtherModels       Bucket
	IncludedSpendUSD  float64
	BonusSpendUSD     float64
	TotalSpendUSD     float64
	OnDemandSpendUSD  float64
	AutoPercentUsed   float64
	APIPercentUsed    float64
	TotalPercentUsed  float64
}

type Bucket struct {
	UsedUSD           *float64
	UsagePercent      float64
	EstimatedTotalUSD *float64
	GuaranteedUSD     *float64
}

type usageSummaryResponse struct {
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`
	MembershipType    string `json:"membershipType"`
	AutoModelMessage  string `json:"autoModelSelectedDisplayMessage"`
	NamedModelMessage string `json:"namedModelSelectedDisplayMessage"`
	IndividualUsage   struct {
		Plan struct {
			Limit            flexibleNumber `json:"limit"`
			AutoPercentUsed  flexibleNumber `json:"autoPercentUsed"`
			APIPercentUsed   flexibleNumber `json:"apiPercentUsed"`
			TotalPercentUsed flexibleNumber `json:"totalPercentUsed"`
			Breakdown        struct {
				Included flexibleNumber `json:"included"`
				Bonus    flexibleNumber `json:"bonus"`
				Total    flexibleNumber `json:"total"`
			} `json:"breakdown"`
		} `json:"plan"`
		OnDemand struct {
			Used flexibleNumber `json:"used"`
		} `json:"onDemand"`
	} `json:"individualUsage"`
}

type periodUsageResponse struct {
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`
	DisplayMessage    string `json:"displayMessage"`
	AutoModelMessage  string `json:"autoModelSelectedDisplayMessage"`
	NamedModelMessage string `json:"namedModelSelectedDisplayMessage"`
	PlanUsage         struct {
		TotalSpend       flexibleNumber `json:"totalSpend"`
		IncludedSpend    flexibleNumber `json:"includedSpend"`
		BonusSpend       flexibleNumber `json:"bonusSpend"`
		Limit            flexibleNumber `json:"limit"`
		AutoPercentUsed  flexibleNumber `json:"autoPercentUsed"`
		APIPercentUsed   flexibleNumber `json:"apiPercentUsed"`
		TotalPercentUsed flexibleNumber `json:"totalPercentUsed"`
	} `json:"planUsage"`
	SpendLimitUsage struct {
		TotalSpend     flexibleNumber `json:"totalSpend"`
		IndividualUsed flexibleNumber `json:"individualUsed"`
	} `json:"spendLimitUsage"`
}

type aggregatedUsageResponse struct {
	Aggregations []aggregatedUsageRow `json:"aggregations"`
}

type aggregatedUsageRow struct {
	ModelIntent string         `json:"modelIntent"`
	TotalCents  flexibleNumber `json:"totalCents"`
	Tier        int            `json:"tier"`
}

type aggregatedUsageRequest struct {
	TeamID    int   `json:"teamId"`
	StartDate int64 `json:"startDate"`
}

type flexibleNumber struct {
	value float64
	set   bool
}

func (number *flexibleNumber) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return err
		}
		number.value = value
		number.set = true
		return nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	number.value = value
	number.set = true
	return nil
}

func NewClient(config Config) (*Client, error) {
	base := strings.TrimSpace(config.BaseURL)
	if base == "" {
		base = defaultBaseURL
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse Cursor dashboard URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, errors.New("Cursor dashboard URL must be HTTP or HTTPS")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout, CheckRedirect: rejectRedirects}
	} else if httpClient.CheckRedirect == nil {
		cloned := *httpClient
		cloned.CheckRedirect = rejectRedirects
		httpClient = &cloned
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}, nil
}

func rejectRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func (client *Client) Fetch(ctx context.Context, accessToken, accountID string) (Snapshot, error) {
	accessToken = strings.TrimSpace(accessToken)
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" {
		return Snapshot{}, errors.New("Cursor access token is required")
	}
	if accountID == "" {
		return Snapshot{}, errors.New("Cursor account ID is required")
	}
	cookie := sessionCookie(accountID, accessToken)
	summary, summaryErr := getJSON[usageSummaryResponse](ctx, client, http.MethodGet, "/api/usage-summary", cookie, nil)
	period, periodErr := getJSON[periodUsageResponse](ctx, client, http.MethodPost, "/api/dashboard/get-current-period-usage", cookie, []byte("{}"))
	if summaryErr != nil && periodErr != nil {
		return Snapshot{}, fmt.Errorf("read Cursor usage: %w", errors.Join(summaryErr, periodErr))
	}
	snapshot := Snapshot{}
	if summaryErr == nil {
		applySummary(&snapshot, summary)
	}
	if periodErr == nil {
		applyPeriod(&snapshot, period)
	}
	startDate := snapshot.BillingCycleStart.UnixMilli()
	if startDate > 0 {
		body, err := json.Marshal(aggregatedUsageRequest{TeamID: individualTeamID, StartDate: startDate})
		if err != nil {
			return Snapshot{}, fmt.Errorf("encode Cursor usage-events request: %w", err)
		}
		events, eventsErr := getJSON[aggregatedUsageResponse](ctx, client, http.MethodPost, "/api/dashboard/get-aggregated-usage-events", cookie, body)
		if eventsErr == nil {
			applyAggregations(&snapshot, events)
		}
	}
	return snapshot, nil
}

func applySummary(snapshot *Snapshot, summary usageSummaryResponse) {
	snapshot.MembershipType = strings.TrimSpace(summary.MembershipType)
	snapshot.BillingCycleStart = parseDashboardTime(summary.BillingCycleStart)
	snapshot.BillingCycleEnd = parseDashboardTime(summary.BillingCycleEnd)
	snapshot.AutoModelMessage = strings.TrimSpace(summary.AutoModelMessage)
	snapshot.NamedModelMessage = strings.TrimSpace(summary.NamedModelMessage)
	snapshot.AutoPercentUsed = summary.IndividualUsage.Plan.AutoPercentUsed.value
	snapshot.APIPercentUsed = summary.IndividualUsage.Plan.APIPercentUsed.value
	snapshot.TotalPercentUsed = summary.IndividualUsage.Plan.TotalPercentUsed.value
	snapshot.IncludedSpendUSD = centsToUSD(summary.IndividualUsage.Plan.Breakdown.Included.value)
	snapshot.BonusSpendUSD = centsToUSD(summary.IndividualUsage.Plan.Breakdown.Bonus.value)
	snapshot.TotalSpendUSD = centsToUSD(summary.IndividualUsage.Plan.Breakdown.Total.value)
	snapshot.OnDemandSpendUSD = centsToUSD(summary.IndividualUsage.OnDemand.Used.value)
	snapshot.CursorModels.UsagePercent = snapshot.AutoPercentUsed
	snapshot.OtherModels.UsagePercent = snapshot.APIPercentUsed
	guaranteed := centsToUSD(summary.IndividualUsage.Plan.Limit.value)
	snapshot.OtherModels.GuaranteedUSD = floatPointer(guaranteed)
}

func applyPeriod(snapshot *Snapshot, period periodUsageResponse) {
	if snapshot.BillingCycleStart.IsZero() {
		snapshot.BillingCycleStart = parseDashboardTime(period.BillingCycleStart)
	}
	if snapshot.BillingCycleEnd.IsZero() {
		snapshot.BillingCycleEnd = parseDashboardTime(period.BillingCycleEnd)
	}
	if snapshot.DisplayMessage == "" {
		snapshot.DisplayMessage = strings.TrimSpace(period.DisplayMessage)
	}
	if snapshot.AutoModelMessage == "" {
		snapshot.AutoModelMessage = strings.TrimSpace(period.AutoModelMessage)
	}
	if snapshot.NamedModelMessage == "" {
		snapshot.NamedModelMessage = strings.TrimSpace(period.NamedModelMessage)
	}
	if snapshot.AutoPercentUsed == 0 {
		snapshot.AutoPercentUsed = period.PlanUsage.AutoPercentUsed.value
		snapshot.CursorModels.UsagePercent = snapshot.AutoPercentUsed
	}
	if snapshot.APIPercentUsed == 0 {
		snapshot.APIPercentUsed = period.PlanUsage.APIPercentUsed.value
		snapshot.OtherModels.UsagePercent = snapshot.APIPercentUsed
	}
	if snapshot.TotalPercentUsed == 0 {
		snapshot.TotalPercentUsed = period.PlanUsage.TotalPercentUsed.value
	}
	if snapshot.IncludedSpendUSD == 0 {
		snapshot.IncludedSpendUSD = centsToUSD(period.PlanUsage.IncludedSpend.value)
	}
	if snapshot.BonusSpendUSD == 0 {
		snapshot.BonusSpendUSD = centsToUSD(period.PlanUsage.BonusSpend.value)
	}
	if snapshot.TotalSpendUSD == 0 {
		snapshot.TotalSpendUSD = centsToUSD(period.PlanUsage.TotalSpend.value)
	}
	if snapshot.OnDemandSpendUSD == 0 {
		snapshot.OnDemandSpendUSD = centsToUSD(period.SpendLimitUsage.IndividualUsed.value)
		if snapshot.OnDemandSpendUSD == 0 {
			snapshot.OnDemandSpendUSD = centsToUSD(period.SpendLimitUsage.TotalSpend.value)
		}
	}
	if snapshot.OtherModels.GuaranteedUSD == nil {
		guaranteed := centsToUSD(period.PlanUsage.Limit.value)
		snapshot.OtherModels.GuaranteedUSD = floatPointer(guaranteed)
	}
}

func applyAggregations(snapshot *Snapshot, events aggregatedUsageResponse) {
	var cursorUsed, otherUsed float64
	var hasCursor, hasOther bool
	for _, row := range events.Aggregations {
		if strings.TrimSpace(row.ModelIntent) == "" {
			continue
		}
		cost := centsToUSD(row.TotalCents.value)
		switch row.Tier {
		case cursorModelTier:
			cursorUsed += cost
			hasCursor = true
		case otherModelTier:
			otherUsed += cost
			hasOther = true
		}
	}
	if hasCursor {
		snapshot.CursorModels.UsedUSD = floatPointer(cursorUsed)
		snapshot.CursorModels.EstimatedTotalUSD = estimatedTotal(cursorUsed, snapshot.AutoPercentUsed)
	}
	if hasOther {
		snapshot.OtherModels.UsedUSD = floatPointer(otherUsed)
	}
}

func estimatedTotal(used, percent float64) *float64 {
	if percent <= 0 || percent >= 100 {
		return nil
	}
	return floatPointer(used * 100 / percent)
}

func centsToUSD(cents float64) float64 {
	return cents / 100
}

func floatPointer(value float64) *float64 {
	copied := value
	return &copied
}

func sessionCookie(accountID, accessToken string) string {
	return sessionCookieName + "=" + url.QueryEscape(accountID+"::"+accessToken)
}

func parseDashboardTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if millis, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if millis > 1_000_000_000_000 {
			return time.UnixMilli(millis).UTC()
		}
		return time.Unix(millis, 0).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func getJSON[T any](ctx context.Context, client *Client, method, path, cookie string, body []byte) (T, error) {
	var zero T
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.endpoint(path), reader)
	if err != nil {
		return zero, fmt.Errorf("create Cursor %s request: %w", path, err)
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("cookie", cookie)
	request.Header.Set("origin", cursorOrigin)
	request.Header.Set("referer", cursorDashboardReferrer)
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return zero, fmt.Errorf("request Cursor %s: %w", path, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxUsageResponseBytes+1))
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return zero, fmt.Errorf("read Cursor %s: %w", path, err)
	}
	if len(raw) > maxUsageResponseBytes {
		return zero, fmt.Errorf("Cursor %s response exceeds 1 MiB", path)
	}
	if response.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("Cursor %s returned HTTP %d", path, response.StatusCode)
	}
	var decoded T
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return zero, fmt.Errorf("decode Cursor %s: %w", path, err)
	}
	return decoded, nil
}

func (client *Client) endpoint(path string) string {
	return strings.TrimRight(client.baseURL.String(), "/") + path
}
