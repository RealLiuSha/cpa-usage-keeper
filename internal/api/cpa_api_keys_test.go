package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"

	"gorm.io/gorm"
)

type keyPolicyRouteItem struct {
	APIKey         string `json:"apiKey"`
	DisplayKey     string `json:"displayKey"`
	Label          string `json:"label"`
	SourceType     string `json:"sourceType"`
	AccessProvider string `json:"accessProvider"`
	Enabled        bool   `json:"enabled"`
}

func TestCPAAPIKeyRoutesExposeKeyPolicyCatalogRows(t *testing.T) {
	db := openCPAAPIKeyAPITestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	if err := repository.SyncCPAAPIKeys(db, []string{"sk-alpha123456"}, syncedAt); err != nil {
		t.Fatalf("seed native: %v", err)
	}
	if err := repository.SyncPluginKeyPolicyKeys(db, []repository.PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: false, KeyPreview: "cpa_Ab…xy12"},
	}, syncedAt); err != nil {
		t.Fatalf("seed plugin: %v", err)
	}
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{CPAAPIKeys: service.NewCPAAPIKeyService(db)})

	for _, path := range []string{"/api/v1/usage/api-keys", "/api/v1/usage/api-keys/settings"} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d body=%s", path, resp.Code, resp.Body.String())
		}
		var parsed struct {
			Items []keyPolicyRouteItem `json:"items"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		if len(parsed.Items) != 2 {
			t.Fatalf("%s: expected native and plugin rows, got %+v", path, parsed.Items)
		}
		native, plugin := parsed.Items[0], parsed.Items[1]
		if native.SourceType != entities.CPAAPIKeySourceNative || !native.Enabled {
			t.Fatalf("%s: unexpected native row: %+v", path, native)
		}
		// The catalog's disabled state is surfaced, and the logical id is never handed out as a key.
		if plugin.SourceType != entities.CPAAPIKeySourcePlugin || plugin.AccessProvider != entities.CPAAPIKeyAccessProviderKeyPolicy ||
			plugin.Enabled || plugin.APIKey != "" || plugin.Label != "Team A" || plugin.DisplayKey != "cpa_Ab…xy12" {
			t.Fatalf("%s: unexpected plugin row: %+v", path, plugin)
		}
	}
}

func TestAuthAPIKeyLoginRejectsPluginLogicalIDAndAcceptsNative(t *testing.T) {
	db := openCPAAPIKeyAPITestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	if err := repository.SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, syncedAt); err != nil {
		t.Fatalf("seed native: %v", err)
	}
	if err := repository.SyncPluginKeyPolicyKeys(db, []repository.PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, syncedAt); err != nil {
		t.Fatalf("seed plugin: %v", err)
	}

	sessions := auth.NewSessionManager(time.Hour)
	config := AuthConfig{Enabled: true, LoginPassword: "secret", SessionTTL: time.Hour}
	router := NewRouter(nil, nil, nil, nil, config, NewAuthHandler(config, sessions), "", OptionalProviders{
		CPAAPIKeys: service.NewCPAAPIKeyService(db),
	})

	for _, tc := range []struct {
		apiKey string
		want   int
	}{
		{apiKey: "team-a", want: http.StatusUnauthorized},
		{apiKey: "sk-native-alpha", want: http.StatusNoContent},
	} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/api-key-login", bytes.NewBufferString(`{"apiKey":"`+tc.apiKey+`"}`))
		req.Header.Set(requestIntentHeaderName, requestIntentHeaderValueFetch)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(resp, req)
		if resp.Code != tc.want {
			t.Fatalf("login with %q: status=%d want %d body=%s", tc.apiKey, resp.Code, tc.want, strings.TrimSpace(resp.Body.String()))
		}
	}
}

func openCPAAPIKeyAPITestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "api-keys.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
