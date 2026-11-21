package repository

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

// Gate A: native sync must not soft-delete plugin rows (H1 / fact-2).
func TestSyncCPAAPIKeysDoesNotSoftDeletePluginRows(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, syncedAt); err != nil {
		t.Fatalf("native seed: %v", err)
	}
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true, KeyPreview: "cpa_Ab…xy12"},
	}, syncedAt); err != nil {
		t.Fatalf("plugin seed: %v", err)
	}

	// Native-only resync with a different key list.
	if err := SyncCPAAPIKeys(db, []string{"sk-native-beta"}, syncedAt.Add(time.Hour)); err != nil {
		t.Fatalf("native resync: %v", err)
	}

	var plugin entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin row: %v", err)
	}
	if plugin.IsDeleted || !plugin.IsKeyPolicyCatalog() {
		t.Fatalf("plugin row must remain active after native-only sync, got %+v", plugin)
	}

	var nativeAlpha entities.CPAAPIKey
	if err := db.Where("api_key = ?", "sk-native-alpha").First(&nativeAlpha).Error; err != nil {
		t.Fatalf("load stale native: %v", err)
	}
	if !nativeAlpha.IsDeleted {
		t.Fatalf("native missing from list must be soft-deleted, got %+v", nativeAlpha)
	}
}

// Gate B: plugin sync (including empty/absent) must not soft-delete native rows (H2 / fact-3).
func TestSyncPluginKeyPolicyKeysDoesNotSoftDeleteNativeRows(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, syncedAt); err != nil {
		t.Fatalf("native seed: %v", err)
	}
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, syncedAt); err != nil {
		t.Fatalf("plugin seed: %v", err)
	}

	// Empty ready catalog soft-deletes plugin rows only.
	if err := SyncPluginKeyPolicyKeys(db, nil, syncedAt.Add(time.Hour)); err != nil {
		t.Fatalf("plugin empty sync: %v", err)
	}

	var native entities.CPAAPIKey
	if err := db.Where("api_key = ?", "sk-native-alpha").First(&native).Error; err != nil {
		t.Fatalf("load native: %v", err)
	}
	if native.IsDeleted {
		t.Fatalf("native row must stay active after plugin empty sync: %+v", native)
	}

	var plugin entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if !plugin.IsDeleted {
		t.Fatalf("plugin missing from catalog must be soft-deleted: %+v", plugin)
	}

	// SoftDeleteAllPluginKeyPolicyKeys is the absent path; still must not touch native.
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, syncedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("plugin revive seed: %v", err)
	}
	if err := SoftDeleteAllPluginKeyPolicyKeys(db, syncedAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("soft-delete all plugin: %v", err)
	}
	if err := db.Where("api_key = ?", "sk-native-alpha").First(&native).Error; err != nil {
		t.Fatalf("reload native: %v", err)
	}
	if native.IsDeleted {
		t.Fatalf("native must remain after SoftDeleteAllPluginKeyPolicyKeys: %+v", native)
	}
}

// Gate C / C2: soft-deleted plugin rows revive with same local id; alias preserved (H7 / fact-4).
func TestSyncPluginKeyPolicyKeysRevivesSoftDeletedRowWithSameID(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true, KeyPreview: "cpa_Ab…xy12"},
	}, syncedAt); err != nil {
		t.Fatalf("initial plugin sync: %v", err)
	}
	var first entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&first).Error; err != nil {
		t.Fatalf("load first row: %v", err)
	}
	if err := UpdateCPAAPIKeyAlias(db, first.ID, "User Alias"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	if err := SoftDeleteAllPluginKeyPolicyKeys(db, syncedAt.Add(time.Hour)); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A Renamed", Enabled: false, KeyPreview: "cpa_New…view"},
	}, syncedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("revive sync: %v", err)
	}

	var revived entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&revived).Error; err != nil {
		t.Fatalf("load revived: %v", err)
	}
	if revived.ID != first.ID {
		t.Fatalf("local id must stay stable on revive: first=%d revived=%d", first.ID, revived.ID)
	}
	if revived.IsDeleted {
		t.Fatalf("revived row must be active: %+v", revived)
	}
	if revived.KeyAlias != "User Alias" {
		t.Fatalf("non-empty alias must not be overwritten, got %q", revived.KeyAlias)
	}
	if revived.Enabled {
		t.Fatalf("enabled=false from catalog must stick, got %+v", revived)
	}
	if revived.DisplayKey != "cpa_New…view" || revived.ExternalID != "team-a" {
		t.Fatalf("meta not updated on revive: %+v", revived)
	}

	// Full-table lookup: soft-deleted occupancy must not cause unique-index INSERT conflicts.
	var count int64
	if err := db.Model(&entities.CPAAPIKey{}).Where("api_key = ?", "team-a").Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected single row for api_key after revive, got %d", count)
	}
}

// Gate: empty/NULL source historical rows stay under native management (H8 / fact-6).
// Gate: plugin logical id must not authenticate (H4 / fact-7 repository side).
func TestFindActiveCPAAPIKeyByValueRejectsPluginIDs(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Team A", Enabled: true},
	}, syncedAt); err != nil {
		t.Fatalf("plugin seed: %v", err)
	}
	if err := SyncCPAAPIKeys(db, []string{"sk-native-alpha"}, syncedAt); err != nil {
		t.Fatalf("native seed: %v", err)
	}

	_, err := FindActiveCPAAPIKeyByValue(db, "team-a")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("plugin id must not login, got %v", err)
	}
	row, err := FindActiveCPAAPIKeyByValue(db, "sk-native-alpha")
	if err != nil || row.APIKey != "sk-native-alpha" {
		t.Fatalf("native login must succeed: row=%+v err=%v", row, err)
	}
}

func TestSyncPluginKeyPolicyKeysSkipsNativeOwnedAPIKey(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// First-come-first-served: plugin must not flip a pre-existing native row.
	if err := SyncCPAAPIKeys(db, []string{"team-a"}, syncedAt); err != nil {
		t.Fatalf("native seed colliding id: %v", err)
	}
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-a", Name: "Plugin Team", Enabled: true},
	}, syncedAt.Add(time.Hour)); err != nil {
		t.Fatalf("plugin sync: %v", err)
	}

	var row entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-a").First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.SourceType != entities.CPAAPIKeySourceNative {
		t.Fatalf("first-come-first-served: source must stay native, got %+v", row)
	}
	if row.KeyAlias == "Plugin Team" {
		t.Fatalf("plugin must not overwrite native alias")
	}
}

// First-come-first-served (symmetric): native must not flip a pre-existing plugin row.
func TestSyncCPAAPIKeysSkipsPluginOwnedAPIKey(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "shared-id", Name: "Plugin Owner", Enabled: true, KeyPreview: "cpa_xx"},
	}, syncedAt); err != nil {
		t.Fatalf("plugin seed: %v", err)
	}
	if err := SyncCPAAPIKeys(db, []string{"shared-id", "sk-other"}, syncedAt.Add(time.Hour)); err != nil {
		t.Fatalf("native sync: %v", err)
	}

	var shared entities.CPAAPIKey
	if err := db.Where("api_key = ?", "shared-id").First(&shared).Error; err != nil {
		t.Fatalf("load shared: %v", err)
	}
	if !shared.IsKeyPolicyCatalog() {
		t.Fatalf("plugin occupancy must not be flipped to native: %+v", shared)
	}
	if shared.KeyAlias != "Plugin Owner" {
		t.Fatalf("native must not overwrite plugin alias: %+v", shared)
	}
	// Unrelated native key still lands.
	var other entities.CPAAPIKey
	if err := db.Where("api_key = ?", "sk-other").First(&other).Error; err != nil {
		t.Fatalf("expected other native key: %v", err)
	}
	if other.SourceType != entities.CPAAPIKeySourceNative {
		t.Fatalf("unrelated native key: %+v", other)
	}
}

func TestSyncPluginKeyPolicyKeysCreatesIdentityFromCatalog(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "  team-a  ", Name: "  Team A  ", Enabled: true, KeyPreview: "cpa_Ab…xy12"},
		{ID: "", Name: "skip empty"},
		{ID: "team-a", Name: "dup ignored"},
	}, syncedAt); err != nil {
		t.Fatalf("plugin sync: %v", err)
	}

	rows, err := ListActiveCPAAPIKeys(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one active plugin row, got %+v", rows)
	}
	row := rows[0]
	if row.APIKey != "team-a" || row.ExternalID != "team-a" {
		t.Fatalf("match key must be trimmed logical id, got %+v", row)
	}
	if !row.IsKeyPolicyCatalog() {
		t.Fatalf("source: got %q %q", row.SourceType, row.AccessProvider)
	}
	if row.KeyAlias != "Team A" || row.DisplayKey != "cpa_Ab…xy12" || !row.Enabled {
		t.Fatalf("catalog fields: %+v", row)
	}
}

// Enabled=false must land in a single Create (no post-Create Update workaround).
func TestSyncPluginKeyPolicyKeysCreatesDisabledKeysWithoutDefaultTag(t *testing.T) {
	db := openCPAAPIKeyTestDatabase(t)
	syncedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	if err := SyncPluginKeyPolicyKeys(db, []PluginKeyPolicyKey{
		{ID: "team-off", Name: "Off", Enabled: false, KeyPreview: "cpa_off"},
		{ID: "team-on", Name: "On", Enabled: true, KeyPreview: "cpa_on"},
	}, syncedAt); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var off entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-off").First(&off).Error; err != nil {
		t.Fatalf("load off: %v", err)
	}
	if off.Enabled {
		t.Fatalf("enabled=false must persist on Create, got %+v", off)
	}
	var raw int
	if err := db.Raw("SELECT enabled FROM cpa_api_keys WHERE api_key = ?", "team-off").Scan(&raw).Error; err != nil {
		t.Fatalf("raw enabled: %v", err)
	}
	if raw != 0 {
		t.Fatalf("sqlite enabled column must be 0, got %d", raw)
	}
	var on entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-on").First(&on).Error; err != nil {
		t.Fatalf("load on: %v", err)
	}
	if !on.Enabled {
		t.Fatalf("enabled=true must persist, got %+v", on)
	}
}

func openCPAAPIKeyTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "cpa-api-key.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return db
}
