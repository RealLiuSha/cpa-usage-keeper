package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// addCPAAPIKeysSourceMigration adds dual-source identity fields for native and
// plugin:cpa-key-policy keys, backfills historical rows, and indexes (source, is_deleted).
func addCPAAPIKeysSourceMigration(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("database is nil")
	}
	migrator := tx.Migrator()
	if !migrator.HasColumn(&entities.CPAAPIKey{}, "Source") {
		if err := migrator.AddColumn(&entities.CPAAPIKey{}, "Source"); err != nil {
			return fmt.Errorf("add CPA API key source: %w", err)
		}
	}
	if !migrator.HasColumn(&entities.CPAAPIKey{}, "ExternalID") {
		if err := migrator.AddColumn(&entities.CPAAPIKey{}, "ExternalID"); err != nil {
			return fmt.Errorf("add CPA API key external_id: %w", err)
		}
	}
	if !migrator.HasColumn(&entities.CPAAPIKey{}, "Enabled") {
		if err := migrator.AddColumn(&entities.CPAAPIKey{}, "Enabled"); err != nil {
			return fmt.Errorf("add CPA API key enabled: %w", err)
		}
	}
	// Source may already be "native" from the column default after AddColumn; always
	// normalize empty values. Enabled backfill only touches native-predicate rows so an
	// idempotent re-run never forces plugin catalog keys back to enabled=1.
	if err := tx.Exec(
		`UPDATE cpa_api_keys SET source = ? WHERE source IS NULL OR source = ''`,
		entities.CPAAPIKeySourceNative,
	).Error; err != nil {
		return fmt.Errorf("backfill CPA API key source: %w", err)
	}
	if err := tx.Exec(
		`UPDATE cpa_api_keys SET enabled = 1 WHERE source = ? OR source IS NULL OR source = ''`,
		entities.CPAAPIKeySourceNative,
	).Error; err != nil {
		return fmt.Errorf("backfill CPA API key enabled: %w", err)
	}
	if !migrator.HasIndex(&entities.CPAAPIKey{}, "idx_cpa_api_keys_source_is_deleted") {
		if err := migrator.CreateIndex(&entities.CPAAPIKey{}, "idx_cpa_api_keys_source_is_deleted"); err != nil {
			return fmt.Errorf("create CPA API key source index: %w", err)
		}
	}
	return nil
}
