package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const quotaUnavailableReason = "Cursor does not publish a subscription remaining-quota API"

type hostAuthFile struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Provider  string `json:"provider"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	Success   int64  `json:"success"`
	Failed    int64  `json:"failed"`
}

type hostAuthListResponse struct {
	Files []hostAuthFile `json:"files"`
}

type hostAuthGetResponse struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name"`
	JSON      json.RawMessage `json:"json"`
}

type cursorModelStatus struct {
	ID       string `json:"id"`
	Disabled bool   `json:"disabled"`
}

type cursorQuotaStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type cursorAccountStatus struct {
	AuthIndex         string              `json:"auth_index"`
	Name              string              `json:"name"`
	Label             string              `json:"label"`
	Status            string              `json:"status"`
	Success           int64               `json:"success"`
	Failed            int64               `json:"failed"`
	SubscriptionQuota cursorQuotaStatus   `json:"subscription_quota"`
	LocalUsage        localUsageStatus    `json:"local_usage"`
	Models            []cursorModelStatus `json:"models"`
}

type cursorManagementStatus struct {
	Provider    string                `json:"provider"`
	GeneratedAt time.Time             `json:"generated_at"`
	Accounts    []cursorAccountStatus `json:"accounts"`
}

type disabledModelsUpdate struct {
	AuthIndex      string   `json:"auth_index"`
	DisabledModels []string `json:"disabled_models"`
}

func (handler *Handler) managementStatus(ctx context.Context) (managementResponse, error) {
	files, err := handler.cursorAuthFiles(ctx)
	if err != nil {
		return managementError(http.StatusBadGateway, err.Error()), nil
	}
	status := cursorManagementStatus{Provider: "cursor", GeneratedAt: time.Now().UTC(), Accounts: make([]cursorAccountStatus, 0, len(files))}
	for _, file := range files {
		account, accountErr := handler.cursorAccountStatus(ctx, file)
		if accountErr != nil {
			account = cursorAccountStatus{
				AuthIndex:         file.AuthIndex,
				Name:              file.Name,
				Label:             file.Label,
				Status:            "unavailable: " + accountErr.Error(),
				SubscriptionQuota: cursorQuotaStatus{Status: "unavailable", Reason: quotaUnavailableReason},
				Models:            []cursorModelStatus{},
			}
		}
		status.Accounts = append(status.Accounts, account)
	}
	return managementJSON(http.StatusOK, status)
}

func (handler *Handler) cursorAccountStatus(ctx context.Context, file hostAuthFile) (cursorAccountStatus, error) {
	credential, err := handler.getCursorCredential(ctx, file.AuthIndex)
	if err != nil {
		return cursorAccountStatus{}, err
	}
	models, err := handler.cursor.DiscoverModels(ctx, credential.AccessToken)
	if err != nil {
		return cursorAccountStatus{}, fmt.Errorf("discover Cursor models: %w", err)
	}
	disabled := normalizedModelSet(credential.DisabledModels)
	items := make([]cursorModelStatus, 0, len(models))
	for _, model := range models {
		id := normalizeModelID(model)
		if id == "" {
			continue
		}
		_, blocked := disabled[id]
		items = append(items, cursorModelStatus{ID: id, Disabled: blocked})
	}
	return cursorAccountStatus{
		AuthIndex:         file.AuthIndex,
		Name:              file.Name,
		Label:             file.Label,
		Status:            file.Status,
		Success:           file.Success,
		Failed:            file.Failed,
		SubscriptionQuota: cursorQuotaStatus{Status: "unavailable", Reason: quotaUnavailableReason},
		LocalUsage:        handler.usage.snapshot(file.AuthIndex),
		Models:            items,
	}, nil
}

func (handler *Handler) updateDisabledModels(ctx context.Context, body []byte) (managementResponse, error) {
	var update disabledModelsUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return managementError(http.StatusBadRequest, "invalid JSON body"), nil
	}
	update.AuthIndex = strings.TrimSpace(update.AuthIndex)
	if update.AuthIndex == "" {
		return managementError(http.StatusBadRequest, "auth_index is required"), nil
	}
	get, credential, err := handler.getCursorAuth(ctx, update.AuthIndex)
	if err != nil {
		return managementError(http.StatusBadGateway, err.Error()), nil
	}
	available, err := handler.cursor.DiscoverModels(ctx, credential.AccessToken)
	if err != nil {
		return managementError(http.StatusBadGateway, "discover Cursor models: "+err.Error()), nil
	}
	known := normalizedModelSet(available)
	disabled := sortedUniqueModels(update.DisabledModels)
	for _, id := range disabled {
		if _, ok := known[id]; !ok {
			return managementError(http.StatusBadRequest, "unknown Cursor model: "+id), nil
		}
	}
	updatedJSON, err := setDisabledModels(get.JSON, disabled)
	if err != nil {
		return managementError(http.StatusInternalServerError, err.Error()), nil
	}
	if _, err := handler.host.Call(ctx, "host.auth.save", struct {
		Name string          `json:"name"`
		JSON json.RawMessage `json:"json"`
	}{Name: get.Name, JSON: updatedJSON}); err != nil {
		return managementError(http.StatusBadGateway, "save Cursor auth: "+err.Error()), nil
	}
	return managementJSON(http.StatusOK, map[string]any{
		"auth_index":      update.AuthIndex,
		"disabled_models": disabled,
	})
}
