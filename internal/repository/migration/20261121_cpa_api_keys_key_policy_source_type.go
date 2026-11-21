package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// legacyKeyPolicySource is the value the fork stored in cpa_api_keys.source before
// upstream introduced source_type/access_provider.
const legacyKeyPolicySource = "plugin:cpa-key-policy"

// keyPolicySourceTypeMigration retires the fork's cpa_api_keys.source column: key-policy
// rows become source_type=plugin with the key-policy access provider (the column default
// from migrationAddPluginAPIKeySources left them as native), then the column and its
// index are dropped so source_type is the only source marker.
func keyPolicySourceTypeMigration(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("cpa_api_keys") || !tx.Migrator().HasColumn("cpa_api_keys", "source") {
		return nil
	}
	if err := tx.Exec(
		`UPDATE cpa_api_keys SET source_type = ?, access_provider = ? WHERE source = ?`,
		entities.CPAAPIKeySourcePlugin, entities.CPAAPIKeyAccessProviderKeyPolicy, legacyKeyPolicySource,
	).Error; err != nil {
		return fmt.Errorf("move key-policy API key source: %w", err)
	}
	if err := tx.Exec(`DROP INDEX IF EXISTS idx_cpa_api_keys_source_is_deleted`).Error; err != nil {
		return fmt.Errorf("drop CPA API key source index: %w", err)
	}
	return dropColumnIfExists(tx, "cpa_api_keys", "source", "cpa_api_keys")
}
