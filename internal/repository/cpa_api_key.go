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

	var existingRows []struct {
		ID           int64
		APIKey       string
		DisplayKey   string
		Source       string
		Enabled      bool
		IsDeleted    bool
		LastSyncedAt *time.Time `gorm:"serializer:storageTime"`
	}
	// Compare the full native/plugin identity set through the reader before opening a writer transaction.
	if err := db.Clauses(dbresolver.Read).Model(&entities.CPAAPIKey{}).
		Select("id, api_key, display_key, source, enabled, is_deleted, last_synced_at").Find(&existingRows).Error; err != nil {
		return err
	}
	type keyUpdate struct {
		id     int64
		fields map[string]any
	}
	existingByKey := make(map[string]int, len(existingRows))
	for index, row := range existingRows {
		existingByKey[row.APIKey] = index
	}
	incoming := make(map[string]struct{}, len(uniqueKeys))
	toCreate := make([]entities.CPAAPIKey, 0)
	toUpdate := make([]keyUpdate, 0)
	for _, key := range uniqueKeys {
		incoming[key] = struct{}{}
		if index, ok := existingByKey[key]; ok {
			row := existingRows[index]
			if !entities.IsNativeCPAAPIKeySource(row.Source) {
				// First-writer-wins on the unique api_key: never flip a plugin row to native.
				logrus.WithFields(logrus.Fields{
					"api_key": helper.RedactSensitiveValue(key),
					"source":  row.Source,
				}).Error("native api key sync skipped row owned by non-native source")
				continue
			}
			fields := make(map[string]any)
			if row.Source != entities.CPAAPIKeySourceNative {
				fields["source"] = entities.CPAAPIKeySourceNative
			}
			if !row.Enabled {
				fields["enabled"] = true
			}
			if row.IsDeleted {
				fields["is_deleted"] = false
			}
			if display := helper.RedactSensitiveValue(key); row.DisplayKey != display {
				fields["display_key"] = display
			}
			if len(fields) > 0 {
				fields["last_synced_at"] = &syncedAt
				fields["updated_at"] = syncedAt
				toUpdate = append(toUpdate, keyUpdate{id: row.ID, fields: fields})
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
	staleIDs := make([]int64, 0)
	for _, row := range existingRows {
		if row.IsDeleted || !entities.IsNativeCPAAPIKeySource(row.Source) {
			continue
		}
		if _, ok := incoming[row.APIKey]; !ok {
			staleIDs = append(staleIDs, row.ID)
		}
	}
	if len(toCreate) == 0 && len(toUpdate) == 0 && len(staleIDs) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, change := range toUpdate {
			if err := tx.Model(&entities.CPAAPIKey{}).Where("id = ?", change.id).Updates(change.fields).Error; err != nil {
				return err
			}
		}
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}
		if len(staleIDs) == 0 {
			return nil
		}
		return tx.Model(&entities.CPAAPIKey{}).Where("id IN ?", staleIDs).
			Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
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
		uniqueKeys = append(uniqueKeys, PluginKeyPolicyKey{
			ID:         id,
			Name:       strings.TrimSpace(key.Name),
			Enabled:    key.Enabled,
			KeyPreview: strings.TrimSpace(key.KeyPreview),
		})
	}

	var existingRows []struct {
		ID           int64
		APIKey       string
		Source       string
		KeyAlias     string
		DisplayKey   string
		ExternalID   string
		Enabled      bool
		IsDeleted    bool
		LastSyncedAt *time.Time `gorm:"serializer:storageTime"`
	}
	// Read the complete table before taking the writer so unchanged snapshots can return immediately.
	if err := db.Clauses(dbresolver.Read).Model(&entities.CPAAPIKey{}).
		Select("id, api_key, source, key_alias, display_key, external_id, enabled, is_deleted, last_synced_at").
		Find(&existingRows).Error; err != nil {
		return err
	}
	type pluginUpdate struct {
		id     int64
		fields map[string]any
	}
	existingByKey := make(map[string]int, len(existingRows))
	for index, row := range existingRows {
		existingByKey[row.APIKey] = index
	}
	incoming := make(map[string]struct{}, len(uniqueKeys))
	toCreate := make([]entities.CPAAPIKey, 0)
	toUpdate := make([]pluginUpdate, 0)
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

		if index, ok := existingByKey[key.ID]; ok {
			existing := existingRows[index]
			if !entities.IsPluginKeyPolicySource(existing.Source) {
				// first-writer-wins on uniq api_key; never flip a native/empty-source row to plugin.
				logrus.WithFields(logrus.Fields{"api_key": key.ID, "source": existing.Source}).Error("plugin key-policy sync skipped row owned by non-plugin source")
				continue
			}
			updates := make(map[string]any)
			if existing.DisplayKey != displayKey {
				updates["display_key"] = displayKey
			}
			if existing.ExternalID != key.ID {
				updates["external_id"] = key.ID
			}
			if existing.Enabled != key.Enabled {
				updates["enabled"] = key.Enabled
			}
			if existing.IsDeleted {
				updates["is_deleted"] = false
			}
			if strings.TrimSpace(existing.KeyAlias) == "" && existing.KeyAlias != defaultAlias {
				updates["key_alias"] = defaultAlias
			}
			if len(updates) > 0 {
				updates["last_synced_at"] = &syncedAt
				updates["updated_at"] = syncedAt
				toUpdate = append(toUpdate, pluginUpdate{id: existing.ID, fields: updates})
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
	// Stale scan only manages exact active plugin rows.
	staleIDs := make([]int64, 0)
	for _, row := range existingRows {
		if row.IsDeleted || !entities.IsPluginKeyPolicySource(row.Source) {
			continue
		}
		if _, ok := incoming[row.APIKey]; !ok {
			staleIDs = append(staleIDs, row.ID)
		}
	}
	if len(toCreate) == 0 && len(toUpdate) == 0 && len(staleIDs) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, change := range toUpdate {
			if err := tx.Model(&entities.CPAAPIKey{}).Where("id = ?", change.id).Updates(change.fields).Error; err != nil {
				return err
			}
		}
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}
		if len(staleIDs) == 0 {
			return nil
		}
		return tx.Model(&entities.CPAAPIKey{}).Where("id IN ?", staleIDs).
			Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
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
