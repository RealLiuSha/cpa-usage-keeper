package migration

import (
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestKeyPolicySourceTypeMigrationUpgradesForkSchema replays the fork-era cpa_api_keys
// layout (source/external_id/enabled added by 20260807) through the upstream column
// migration and the source retirement, as a deployed database would see them.
func TestKeyPolicySourceTypeMigrationUpgradesForkSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fork-keys.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	for _, statement := range []string{
		"CREATE TABLE `cpa_api_keys` (`id` integer PRIMARY KEY AUTOINCREMENT,`api_key` text,`display_key` text,`key_alias` text,`is_deleted` numeric,`last_synced_at` datetime,`created_at` datetime,`updated_at` datetime)",
		"CREATE UNIQUE INDEX `uniq_cpa_api_keys_api_key` ON `cpa_api_keys`(`api_key`)",
		"CREATE INDEX `idx_cpa_api_keys_is_deleted` ON `cpa_api_keys`(`is_deleted`)",
		`INSERT INTO cpa_api_keys (id, api_key, display_key, key_alias, is_deleted) VALUES (1, 'sk-native-alpha', 'sk-*********-alpha', '', 0)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
	}
	if err := addCPAAPIKeysSourceMigration(db); err != nil {
		t.Fatalf("addCPAAPIKeysSourceMigration: %v", err)
	}
	for _, statement := range []string{
		// Databases that ran the original 20260807 also carry this index.
		"CREATE INDEX `idx_cpa_api_keys_source_is_deleted` ON `cpa_api_keys`(`source`,`is_deleted`)",
		`INSERT INTO cpa_api_keys (id, api_key, display_key, key_alias, source, external_id, enabled, is_deleted) VALUES (2, 'loboo-1', 'cpa_Ab…xy12', 'Team A', 'plugin:cpa-key-policy', 'loboo-1', 1, 0)`,
		`INSERT INTO cpa_api_keys (id, api_key, display_key, key_alias, source, external_id, enabled, is_deleted) VALUES (3, 'loboo-2', '', 'Off', 'plugin:cpa-key-policy', 'loboo-2', 0, 1)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed key-policy rows: %v", err)
		}
	}

	if err := addPluginAPIKeySourcesMigration(db); err != nil {
		t.Fatalf("addPluginAPIKeySourcesMigration: %v", err)
	}
	if err := keyPolicySourceTypeMigration(db); err != nil {
		t.Fatalf("keyPolicySourceTypeMigration: %v", err)
	}
	// A second run finds no source column and must be a no-op.
	if err := keyPolicySourceTypeMigration(db); err != nil {
		t.Fatalf("second keyPolicySourceTypeMigration: %v", err)
	}

	if db.Migrator().HasColumn("cpa_api_keys", "source") {
		t.Fatal("expected legacy source column to be dropped")
	}
	if db.Migrator().HasIndex("cpa_api_keys", "idx_cpa_api_keys_source_is_deleted") {
		t.Fatal("expected legacy source index to be dropped")
	}
	var rows []entities.CPAAPIKey
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("load rows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %+v", rows)
	}
	if rows[0].SourceType != entities.CPAAPIKeySourceNative || rows[0].AccessProvider != "" || !rows[0].IsEnabled() {
		t.Fatalf("native row changed: %+v", rows[0])
	}
	for index, wantEnabled := range map[int]bool{1: true, 2: false} {
		row := rows[index]
		if !row.IsKeyPolicyCatalog() || row.Enabled != wantEnabled || row.ExternalID != row.APIKey {
			t.Fatalf("key-policy row %d not migrated: %+v", index, row)
		}
	}
	if !rows[2].IsDeleted || rows[1].KeyAlias != "Team A" {
		t.Fatalf("migration must keep other columns: %+v", rows[1:])
	}
}
