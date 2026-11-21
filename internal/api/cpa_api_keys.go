package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/service"
	"cpa-usage-keeper/internal/timeutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxCPAAPIKeyAliasLength = 128

type cpaAPIKeyResponse struct {
	ID             string  `json:"id"`
	KeyAlias       string  `json:"keyAlias"`
	DisplayKey     string  `json:"displayKey"`
	Label          string  `json:"label"`
	LastSyncedAt   *string `json:"lastSyncedAt"`
	SourceType     string  `json:"sourceType"`
	AccessProvider string  `json:"accessProvider"`
	Enabled        bool    `json:"enabled"`
}

type cpaAPIKeyListResponse struct {
	Items []cpaAPIKeyResponse `json:"items"`
}

type cpaAPIKeySettingsResponse struct {
	ID             string  `json:"id"`
	APIKey         string  `json:"apiKey,omitempty"`
	KeyAlias       string  `json:"keyAlias"`
	DisplayKey     string  `json:"displayKey"`
	Label          string  `json:"label"`
	LastSyncedAt   *string `json:"lastSyncedAt"`
	SourceType     string  `json:"sourceType"`
	AccessProvider string  `json:"accessProvider"`
	Enabled        bool    `json:"enabled"`
}

type cpaAPIKeySettingsListResponse struct {
	Items []cpaAPIKeySettingsResponse `json:"items"`
}

type cpaAPIKeyOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type cpaAPIKeyOptionsResponse struct {
	Options []cpaAPIKeyOption `json:"options"`
}

type updateCPAAPIKeyAliasRequest struct {
	KeyAlias string `json:"keyAlias"`
}

// registerPublicCPAAPIKeyOptionRoutes serves the filter candidates to anonymous share
// clients with public-safe labels that never echo raw secrets.
func registerPublicCPAAPIKeyOptionRoutes(router gin.IRoutes, provider service.CPAAPIKeyProvider, usageProvider service.UsageProvider) {
	registerCPAAPIKeyOptionsRoute(router, provider, usageProvider, toPublicCPAAPIKeyOption)
}

// registerCPAAPIKeyOptionsRoute 仅暴露筛选用的 id/label 列表，不含 raw key；label 由调用方决定公开口径。
func registerCPAAPIKeyOptionsRoute(router gin.IRoutes, provider service.CPAAPIKeyProvider, usageProvider service.UsageProvider, toOption func(entities.CPAAPIKey) cpaAPIKeyOption) {
	router.GET("/usage/api-keys/options", func(c *gin.Context) {
		setNoStoreHeaders(c)
		if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
			listScopedCPAAPIKeyOptions(c, usageProvider, scope, toOption)
			return
		}
		rows, err := listCPAAPIKeyOptionRows(c, provider, toOption)
		if err != nil {
			return
		}
		c.JSON(http.StatusOK, cpaAPIKeyOptionsResponse{Options: rows})
	})
}

func registerCPAAPIKeyRoutes(router gin.IRoutes, provider service.CPAAPIKeyProvider, usageProvider service.UsageProvider) {
	router.GET("/usage/api-keys", func(c *gin.Context) {
		rows, err := listCPAAPIKeyRows(c, provider)
		if err != nil {
			return
		}
		c.JSON(http.StatusOK, cpaAPIKeyListResponse{Items: rows})
	})

	router.GET("/usage/api-keys/settings", func(c *gin.Context) {
		rows, err := listCPAAPIKeySettingsRows(c, provider)
		if err != nil {
			return
		}
		c.JSON(http.StatusOK, cpaAPIKeySettingsListResponse{Items: rows})
	})

	registerCPAAPIKeyOptionsRoute(router, provider, usageProvider, toCPAAPIKeyOption)

	router.PATCH("/usage/api-keys/:id", func(c *gin.Context) {
		if provider == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "api key provider is not configured"})
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid api key id"})
			return
		}
		var request updateCPAAPIKeyAliasRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		request.KeyAlias = strings.TrimSpace(request.KeyAlias)
		if err := validateCPAAPIKeyAlias(request.KeyAlias); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		row, err := provider.UpdateCPAAPIKeyAlias(c.Request.Context(), id, request.KeyAlias)
		if err != nil {
			if errors.Is(err, service.ErrInvalidID) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid api key id"})
				return
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
				return
			}
			writeInternalError(c, "update api key alias failed", err)
			return
		}
		c.JSON(http.StatusOK, toCPAAPIKeyResponse(row))
	})
}

func listCPAAPIKeyRows(c *gin.Context, provider service.CPAAPIKeyProvider) ([]cpaAPIKeyResponse, error) {
	if provider == nil {
		return []cpaAPIKeyResponse{}, nil
	}
	rows, err := provider.ListCPAAPIKeys(c.Request.Context())
	if err != nil {
		writeInternalError(c, "list api keys failed", err)
		return nil, err
	}
	response := make([]cpaAPIKeyResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toCPAAPIKeyResponse(row))
	}
	return response, nil
}

func listCPAAPIKeySettingsRows(c *gin.Context, provider service.CPAAPIKeyProvider) ([]cpaAPIKeySettingsResponse, error) {
	if provider == nil {
		return []cpaAPIKeySettingsResponse{}, nil
	}
	rows, err := provider.ListCPAAPIKeys(c.Request.Context())
	if err != nil {
		writeInternalError(c, "list api key settings failed", err)
		return nil, err
	}
	response := make([]cpaAPIKeySettingsResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toCPAAPIKeySettingsResponse(row))
	}
	return response, nil
}

func listCPAAPIKeyOptionRows(c *gin.Context, provider service.CPAAPIKeyProvider, toOption func(entities.CPAAPIKey) cpaAPIKeyOption) ([]cpaAPIKeyOption, error) {
	if provider == nil {
		return []cpaAPIKeyOption{}, nil
	}
	rows, err := provider.ListCPAAPIKeys(c.Request.Context())
	if err != nil {
		writeInternalError(c, "list api key options failed", err)
		return nil, err
	}
	response := make([]cpaAPIKeyOption, 0, len(rows))
	for _, row := range rows {
		response = append(response, toOption(row))
	}
	return response, nil
}

func toCPAAPIKeyResponse(row entities.CPAAPIKey) cpaAPIKeyResponse {
	label := helper.CPAAPIKeyDisplayName(row)
	var lastSyncedAt *string
	if row.LastSyncedAt != nil {
		value := timeutil.FormatStorageTime(*row.LastSyncedAt)
		lastSyncedAt = &value
	}
	return cpaAPIKeyResponse{
		ID:             strconv.FormatInt(row.ID, 10),
		KeyAlias:       row.KeyAlias,
		DisplayKey:     helper.CPAAPIKeyMaskedDisplayKey(row),
		Label:          label,
		LastSyncedAt:   lastSyncedAt,
		SourceType:     cpaAPIKeySourceType(row),
		AccessProvider: row.AccessProvider,
		Enabled:        row.IsEnabled(),
	}
}

func toCPAAPIKeySettingsResponse(row entities.CPAAPIKey) cpaAPIKeySettingsResponse {
	label := helper.CPAAPIKeyDisplayName(row)
	var lastSyncedAt *string
	if row.LastSyncedAt != nil {
		value := timeutil.FormatStorageTime(*row.LastSyncedAt)
		lastSyncedAt = &value
	}
	apiKey := row.APIKey
	if row.SourceType == entities.CPAAPIKeySourcePlugin {
		apiKey = ""
	}
	return cpaAPIKeySettingsResponse{
		ID:             strconv.FormatInt(row.ID, 10),
		APIKey:         apiKey,
		KeyAlias:       row.KeyAlias,
		DisplayKey:     helper.CPAAPIKeyMaskedDisplayKey(row),
		Label:          label,
		LastSyncedAt:   lastSyncedAt,
		SourceType:     cpaAPIKeySourceType(row),
		AccessProvider: row.AccessProvider,
		Enabled:        row.IsEnabled(),
	}
}

func cpaAPIKeySourceType(row entities.CPAAPIKey) string {
	if row.SourceType == entities.CPAAPIKeySourcePlugin {
		return entities.CPAAPIKeySourcePlugin
	}
	return entities.CPAAPIKeySourceNative
}

func toCPAAPIKeyOption(row entities.CPAAPIKey) cpaAPIKeyOption {
	return cpaAPIKeyOption{
		ID:    strconv.FormatInt(row.ID, 10),
		Label: helper.CPAAPIKeyDisplayName(row),
	}
}

func toPublicCPAAPIKeyOption(row entities.CPAAPIKey) cpaAPIKeyOption {
	return cpaAPIKeyOption{
		ID:    strconv.FormatInt(row.ID, 10),
		Label: helper.CPAAPIKeyPublicDisplayName(row),
	}
}

func validateCPAAPIKeyAlias(value string) error {
	if len([]rune(value)) > maxCPAAPIKeyAliasLength {
		return errors.New("keyAlias is too long")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("keyAlias cannot contain control characters")
		}
	}
	return nil
}
