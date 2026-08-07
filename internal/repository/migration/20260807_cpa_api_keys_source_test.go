package migration

import (
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddCPAAPIKeysSourceMigrationAddsColumnsBackfillAndIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "cpa-api-keys-source.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	defer closeOpenedDatabase(t, db)

	if err := createCPAAPIKeysMigration(db); err != nil {
		t.Fatalf("createCPAAPIKeysMigration returned error: %v", err)
	}
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	if err := db.Exec(
		`INSERT INTO cpa_api_keys (id, api_key, display_key, key_alias, is_deleted, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1, "sk-legacy", "sk-*********legacy", "", false, now, now,
	).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := addCPAAPIKeysSourceMigration(db); err != nil {
		t.Fatalf("addCPAAPIKeysSourceMigration returned error: %v", err)
	}
	// Idempotent re-run must not fail.
	if err := addCPAAPIKeysSourceMigration(db); err != nil {
		t.Fatalf("second addCPAAPIKeysSourceMigration returned error: %v", err)
	}

	if !db.Migrator().HasColumn(&entities.CPAAPIKey{}, "Source") {
		t.Fatalf("expected source column")
	}
	if !db.Migrator().HasColumn(&entities.CPAAPIKey{}, "ExternalID") {
		t.Fatalf("expected external_id column")
	}
	if !db.Migrator().HasColumn(&entities.CPAAPIKey{}, "Enabled") {
		t.Fatalf("expected enabled column")
	}
	if !sqliteIndexExists(t, db, "idx_cpa_api_keys_source_is_deleted") {
		t.Fatalf("expected source+is_deleted index")
	}

	var row entities.CPAAPIKey
	if err := db.Where("id = ?", 1).First(&row).Error; err != nil {
		t.Fatalf("load backfilled row: %v", err)
	}
	if row.Source != entities.CPAAPIKeySourceNative {
		t.Fatalf("expected source native after backfill, got %q", row.Source)
	}
	if !row.Enabled {
		t.Fatalf("expected enabled=true after backfill, got %+v", row)
	}

	// After first migration, a disabled plugin row may exist. Idempotent re-run must not force it enabled.
	if err := db.Create(&entities.CPAAPIKey{
		APIKey: "team-disabled", DisplayKey: "cpa_x", KeyAlias: "Off",
		Source: entities.CPAAPIKeySourcePluginKeyPolicy, ExternalID: "team-disabled",
		Enabled: false, IsDeleted: false,
	}).Error; err != nil {
		t.Fatalf("seed disabled plugin: %v", err)
	}
	if err := addCPAAPIKeysSourceMigration(db); err != nil {
		t.Fatalf("third re-run: %v", err)
	}
	var plugin entities.CPAAPIKey
	if err := db.Where("api_key = ?", "team-disabled").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if plugin.Enabled {
		t.Fatalf("idempotent enabled backfill must not reset plugin disabled keys: %+v", plugin)
	}
}
