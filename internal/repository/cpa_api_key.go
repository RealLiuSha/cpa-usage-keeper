package repository

import (
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// cpaAPIKeyNativeSourcePredicate is the single warehouse-wide definition of a
// native CPA API key row (H8). Includes empty/NULL historical sources so login
// and native stale scans stay consistent when backfill is missing.
//
// Usage: db.Where(cpaAPIKeyNativeSourcePredicate, entities.CPAAPIKeySourceNative)
const cpaAPIKeyNativeSourcePredicate = "(source = ? OR source IS NULL OR source = '')"

// PluginKeyPolicyKey is one public catalog entry from cpa-plugin-key-policy.
// ID is the analysis match key written into api_key (never the secret).
type PluginKeyPolicyKey struct {
	ID         string
	Name       string
	Enabled    bool
	KeyPreview string
}

// SyncCPAAPIKeys upserts native management API keys and soft-deletes only native
// rows that are missing from this round (H1). Plugin rows are never soft-deleted here.
//
// Lookup by api_key always scans the full table (including soft-deleted rows)
// because uniq_cpa_api_keys_api_key does not include is_deleted (H7).
func SyncCPAAPIKeys(db *gorm.DB, keys []string, syncedAt time.Time) error {
	seen := make(map[string]struct{}, len(keys))
	uniqueKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniqueKeys = append(uniqueKeys, key)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var existingRows []struct {
			ID        int64
			APIKey    string
			Source    string
			IsDeleted bool
		}
		// Full-table scan: unique index has no is_deleted, so soft-deleted rows still occupy api_key.
		if err := tx.Model(&entities.CPAAPIKey{}).Select("id, api_key, source, is_deleted").Find(&existingRows).Error; err != nil {
			return err
		}

		existingByKey := make(map[string]struct {
			ID        int64
			Source    string
			IsDeleted bool
		}, len(existingRows))
		for _, row := range existingRows {
			existingByKey[row.APIKey] = struct {
				ID        int64
				Source    string
				IsDeleted bool
			}{ID: row.ID, Source: row.Source, IsDeleted: row.IsDeleted}
		}

		incoming := make(map[string]struct{}, len(uniqueKeys))
		toCreate := make([]entities.CPAAPIKey, 0)
		for _, key := range uniqueKeys {
			incoming[key] = struct{}{}
			if existing, ok := existingByKey[key]; ok {
				if !entities.IsNativeCPAAPIKeySource(existing.Source) {
					// first-writer-wins on uniq api_key; see goals/key-policy-review-fixes/decisions.md D1.
					// Never flip a plugin-owned row to native.
					logrus.WithFields(logrus.Fields{
						"api_key": helper.RedactSensitiveValue(key),
						"source":  existing.Source,
					}).Error("native api key sync skipped row owned by non-native source")
					continue
				}
				updates := map[string]any{
					"display_key":    helper.RedactSensitiveValue(key),
					"source":         entities.CPAAPIKeySourceNative,
					"enabled":        true,
					"is_deleted":     false,
					"last_synced_at": &syncedAt,
					"updated_at":     syncedAt,
				}
				if err := tx.Model(&entities.CPAAPIKey{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
					return err
				}
				continue
			}
			toCreate = append(toCreate, entities.CPAAPIKey{
				APIKey:       key,
				DisplayKey:   helper.RedactSensitiveValue(key),
				Source:       entities.CPAAPIKeySourceNative,
				Enabled:      true,
				IsDeleted:    false,
				LastSyncedAt: &syncedAt,
			})
		}
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}

		// Stale scan only manages native rows (H1 / H8).
		staleIDs := make([]int64, 0)
		for _, row := range existingRows {
			if row.IsDeleted {
				continue
			}
			if !entities.IsNativeCPAAPIKeySource(row.Source) {
				continue
			}
			if _, ok := incoming[row.APIKey]; ok {
				continue
			}
			staleIDs = append(staleIDs, row.ID)
		}
		if len(staleIDs) == 0 {
			return nil
		}
		return tx.Model(&entities.CPAAPIKey{}).Where("id IN ?", staleIDs).Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
	})
}

// SyncPluginKeyPolicyKeys upserts plugin:cpa-key-policy catalog rows and soft-deletes
// only plugin rows missing from this ready list (H2). Soft-deleted plugin rows are
// revived in place so local int64 ids stay stable (H7).
//
// Callers must not invoke this on fetch error; only ready catalogs (including empty).
func SyncPluginKeyPolicyKeys(db *gorm.DB, keys []PluginKeyPolicyKey, syncedAt time.Time) error {
	seen := make(map[string]struct{}, len(keys))
	uniqueKeys := make([]PluginKeyPolicyKey, 0, len(keys))
	for _, key := range keys {
		id := strings.TrimSpace(key.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(key.Name)
		preview := strings.TrimSpace(key.KeyPreview)
		uniqueKeys = append(uniqueKeys, PluginKeyPolicyKey{
			ID:         id,
			Name:       name,
			Enabled:    key.Enabled,
			KeyPreview: preview,
		})
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var existingRows []struct {
			ID        int64
			APIKey    string
			Source    string
			KeyAlias  string
			IsDeleted bool
		}
		// Full table (H7): soft-deleted plugin rows must be found and revived, not re-INSERTed.
		if err := tx.Model(&entities.CPAAPIKey{}).Select("id, api_key, source, key_alias, is_deleted").Find(&existingRows).Error; err != nil {
			return err
		}

		existingByKey := make(map[string]struct {
			ID        int64
			Source    string
			KeyAlias  string
			IsDeleted bool
		}, len(existingRows))
		for _, row := range existingRows {
			existingByKey[row.APIKey] = struct {
				ID        int64
				Source    string
				KeyAlias  string
				IsDeleted bool
			}{ID: row.ID, Source: row.Source, KeyAlias: row.KeyAlias, IsDeleted: row.IsDeleted}
		}

		incoming := make(map[string]struct{}, len(uniqueKeys))
		toCreate := make([]entities.CPAAPIKey, 0)
		for _, key := range uniqueKeys {
			incoming[key.ID] = struct{}{}
			displayKey := key.KeyPreview
			if displayKey == "" {
				displayKey = key.ID
			}
			defaultAlias := key.Name
			if defaultAlias == "" {
				defaultAlias = key.ID
			}

			if existing, ok := existingByKey[key.ID]; ok {
				if !entities.IsPluginKeyPolicySource(existing.Source) {
					// first-writer-wins on uniq api_key; see goals/key-policy-review-fixes/decisions.md D1.
					// Never flip a native/empty-source row to plugin.
					logrus.WithFields(logrus.Fields{
						"api_key": key.ID,
						"source":  existing.Source,
					}).Error("plugin key-policy sync skipped row owned by non-plugin source")
					continue
				}
				updates := map[string]any{
					"display_key":    displayKey,
					"external_id":    key.ID,
					"enabled":        key.Enabled,
					"source":         entities.CPAAPIKeySourcePluginKeyPolicy,
					"is_deleted":     false, // H7: revive soft-deleted plugin identity in place
					"last_synced_at": &syncedAt,
					"updated_at":     syncedAt,
				}
				// Do not overwrite a non-empty user alias.
				if strings.TrimSpace(existing.KeyAlias) == "" {
					updates["key_alias"] = defaultAlias
				}
				if err := tx.Model(&entities.CPAAPIKey{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
					return err
				}
				continue
			}

			toCreate = append(toCreate, entities.CPAAPIKey{
				APIKey:       key.ID,
				DisplayKey:   displayKey,
				KeyAlias:     defaultAlias,
				Source:       entities.CPAAPIKeySourcePluginKeyPolicy,
				ExternalID:   key.ID,
				Enabled:      key.Enabled,
				IsDeleted:    false,
				LastSyncedAt: &syncedAt,
			})
		}
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}

		// Stale scan only manages exact plugin:cpa-key-policy rows (H2).
		staleIDs := make([]int64, 0)
		for _, row := range existingRows {
			if row.IsDeleted {
				continue
			}
			if !entities.IsPluginKeyPolicySource(row.Source) {
				continue
			}
			if _, ok := incoming[row.APIKey]; ok {
				continue
			}
			staleIDs = append(staleIDs, row.ID)
		}
		if len(staleIDs) == 0 {
			return nil
		}
		return tx.Model(&entities.CPAAPIKey{}).Where("id IN ?", staleIDs).Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
	})
}

// SoftDeleteAllPluginKeyPolicyKeys marks every active plugin:cpa-key-policy row deleted.
// Used when the plugin endpoint is absent (404/501).
func SoftDeleteAllPluginKeyPolicyKeys(db *gorm.DB, syncedAt time.Time) error {
	return db.Model(&entities.CPAAPIKey{}).
		Where("source = ? AND is_deleted = ?", entities.CPAAPIKeySourcePluginKeyPolicy, false).
		Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
}

func ListActiveCPAAPIKeys(db *gorm.DB) ([]entities.CPAAPIKey, error) {
	var rows []entities.CPAAPIKey
	err := db.Where("is_deleted = ?", false).Order("id asc").Find(&rows).Error
	return rows, err
}

func FindActiveCPAAPIKeyByID(db *gorm.DB, id int64) (entities.CPAAPIKey, error) {
	var row entities.CPAAPIKey
	err := db.Where("id = ? AND is_deleted = ?", id, false).First(&row).Error
	return row, err
}

// FindActiveNativeCPAAPIKeyByValue looks up an active native (or empty-source historical)
// row by api_key for Key Viewer login only (H4 / H8). Plugin logical ids never match.
//
// api_key is a login credential here, not an analysis match key.
func FindActiveNativeCPAAPIKeyByValue(db *gorm.DB, apiKey string) (entities.CPAAPIKey, error) {
	var row entities.CPAAPIKey
	err := db.Where("api_key = ? AND is_deleted = ?", apiKey, false).
		Where(cpaAPIKeyNativeSourcePredicate, entities.CPAAPIKeySourceNative).
		First(&row).Error
	return row, err
}

func UpdateCPAAPIKeyAlias(db *gorm.DB, id int64, keyAlias string) error {
	result := db.Model(&entities.CPAAPIKey{}).Where("id = ? AND is_deleted = ?", id, false).Update("key_alias", strings.TrimSpace(keyAlias))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateCPAAPIKeyLocalRankingProfile 在同一写事务中保存并回读 Key 的本地展示资料。
func UpdateCPAAPIKeyLocalRankingProfile(db *gorm.DB, id int64, keyAlias string, avatarID uint8) (entities.CPAAPIKey, error) {
	var row entities.CPAAPIKey
	err := db.Clauses(dbresolver.Write).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entities.CPAAPIKey{}).Where("id = ?", id).Updates(map[string]any{
			"key_alias":               strings.TrimSpace(keyAlias),
			"local_ranking_avatar_id": avatarID,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("id = ?", id).First(&row).Error
	})
	return row, err
}
