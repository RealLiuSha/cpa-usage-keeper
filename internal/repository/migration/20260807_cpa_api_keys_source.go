package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// cpaAPIKeySourceColumns snapshots the columns this migration added. The source column
// itself is retired by keyPolicySourceTypeMigration, so it no longer exists on the entity.
type cpaAPIKeySourceColumns struct {
	Source     string `gorm:"default:native"`
	ExternalID string
	Enabled    bool
}

func (cpaAPIKeySourceColumns) TableName() string {
	return "cpa_api_keys"
}

// addCPAAPIKeysSourceMigration adds dual-source identity fields for native and
// plugin:cpa-key-policy keys and backfills historical rows. It used to index
// (source, is_deleted) as well; keyPolicySourceTypeMigration drops that index together
// with the column, so databases that have not run this version yet skip creating it.
func addCPAAPIKeysSourceMigration(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("database is nil")
	}
	migrator := tx.Migrator()
	model := &cpaAPIKeySourceColumns{}
	for _, field := range []string{"Source", "ExternalID", "Enabled"} {
		if migrator.HasColumn(model, field) {
			continue
		}
		if err := migrator.AddColumn(model, field); err != nil {
			return fmt.Errorf("add CPA API key %s: %w", field, err)
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
	return nil
}
