package test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/cpaapikeys"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestSyncMetadataKeyPolicyReadyWritesPluginRows(t *testing.T) {
	db := openMetadataTestDatabase(t, "key-policy-ready.db")
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	fetcher := newMetadataTestFetcher()
	enabledFalse := false
	fetcher.keyPolicyKeysResult = &response.KeyPolicyKeysResult{
		StatusCode: http.StatusOK,
		Payload: cpaapikeys.KeyPolicyKeysResponse{
			Keys: []cpaapikeys.KeyPolicyPublicKey{
				{ID: "team-a", Name: "Team A", KeyPreview: "cpa_Ab…xy12"},
				{ID: "team-b", Name: "Team B", Enabled: &enabledFalse},
			},
		},
	}
	fetcher.keyPolicyKeysErr = nil
	fetcher.managementAPIKeysResult = &response.ManagementAPIKeysResult{
		StatusCode: http.StatusOK,
		Payload:    cpaapikeys.ManagementAPIKeysResponse{APIKeys: []string{"sk-native-alpha"}},
	}

	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		BaseURL:         "https://cpa.example.com",
		MetadataFetcher: fetcher,
		Now:             func() time.Time { return now },
	})
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	rows, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected native + 2 plugin rows, got %+v", rows)
	}
	byKey := map[string]entities.CPAAPIKey{}
	for _, row := range rows {
		byKey[row.APIKey] = row
	}
	if byKey["team-a"].Source != entities.CPAAPIKeySourcePluginKeyPolicy || byKey["team-a"].KeyAlias != "Team A" {
		t.Fatalf("team-a: %+v", byKey["team-a"])
	}
	if byKey["team-b"].Enabled {
		t.Fatalf("team-b should be disabled: %+v", byKey["team-b"])
	}
	if byKey["sk-native-alpha"].Source != entities.CPAAPIKeySourceNative {
		t.Fatalf("native: %+v", byKey["sk-native-alpha"])
	}
}

func TestSyncMetadataKeyPolicyAbsentSoftDeletesPluginOnly(t *testing.T) {
	db := openMetadataTestDatabase(t, "key-policy-absent.db")
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	if err := repository.SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, now); err != nil {
		t.Fatalf("seed native: %v", err)
	}
	if err := repository.SyncPluginKeyPolicyKeys(db, []repository.PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, now); err != nil {
		t.Fatalf("seed plugin: %v", err)
	}

	fetcher := newMetadataTestFetcher()
	// Default is already 404 absent.
	fetcher.managementAPIKeysResult = &response.ManagementAPIKeysResult{
		StatusCode: http.StatusOK,
		Payload:    cpaapikeys.ManagementAPIKeysResponse{APIKeys: []string{"sk-native-alpha"}},
	}
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		BaseURL:         "https://cpa.example.com",
		MetadataFetcher: fetcher,
		Now:             func() time.Time { return now.Add(time.Hour) },
	})
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	var plugin entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if !plugin.IsDeleted {
		t.Fatalf("absent must soft-delete plugin: %+v", plugin)
	}
	var native entities.CPAAPIKey
	if err := db.Where("api_key = ?", "sk-native-alpha").First(&native).Error; err != nil {
		t.Fatalf("load native: %v", err)
	}
	if native.IsDeleted {
		t.Fatalf("absent must not soft-delete native: %+v", native)
	}
}

func TestSyncMetadataKeyPolicyErrorDoesNotTouchPluginRows(t *testing.T) {
	db := openMetadataTestDatabase(t, "key-policy-error.db")
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	if err := repository.SyncPluginKeyPolicyKeys(db, []repository.PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, now); err != nil {
		t.Fatalf("seed plugin: %v", err)
	}
	if err := repository.SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, now); err != nil {
		t.Fatalf("seed native: %v", err)
	}

	fetcher := newMetadataTestFetcher()
	fetcher.keyPolicyKeysResult = &response.KeyPolicyKeysResult{StatusCode: http.StatusInternalServerError}
	fetcher.keyPolicyKeysErr = errors.New("key-policy keys: unexpected status 500")
	fetcher.managementAPIKeysResult = &response.ManagementAPIKeysResult{
		StatusCode: http.StatusOK,
		Payload:    cpaapikeys.ManagementAPIKeysResponse{APIKeys: []string{"sk-native-alpha", "sk-native-beta"}},
	}

	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		BaseURL:         "https://cpa.example.com",
		MetadataFetcher: fetcher,
		Now:             func() time.Time { return now.Add(time.Hour) },
	})
	err := syncer.SyncMetadata(context.Background())
	if err == nil || !strings.Contains(err.Error(), "key-policy") {
		t.Fatalf("expected key-policy error to surface, got %v", err)
	}

	var plugin entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if plugin.IsDeleted {
		t.Fatalf("500 must not soft-delete plugin rows: %+v", plugin)
	}
	// Native sync still committed (plugin failure does not roll back native).
	rows, listErr := repository.ListActiveCPAAPIKeys(db)
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	foundBeta := false
	for _, row := range rows {
		if row.APIKey == "sk-native-beta" {
			foundBeta = true
		}
	}
	if !foundBeta {
		t.Fatalf("native sync must still commit when plugin fails: %+v", rows)
	}
}

func TestSyncMetadataKeyPolicyAbsentDoesNotHardFail(t *testing.T) {
	db := openMetadataTestDatabase(t, "key-policy-absent-ok.db")
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	fetcher := newMetadataTestFetcher() // default 404
	fetcher.managementAPIKeysResult = &response.ManagementAPIKeysResult{
		StatusCode: http.StatusOK,
		Payload:    cpaapikeys.ManagementAPIKeysResponse{APIKeys: []string{"sk-native-alpha"}},
	}
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		BaseURL:         "https://cpa.example.com",
		MetadataFetcher: fetcher,
		Now:             func() time.Time { return now },
	})
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatalf("absent plugin must not hard-fail metadata: %v", err)
	}
	rows, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].APIKey != "sk-native-alpha" {
		t.Fatalf("expected only native key, got %+v", rows)
	}
}
