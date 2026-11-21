package repository

import (
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

// PluginKeyPolicyKey is one public catalog entry from cpa-plugin-key-policy.
// ID is the analysis match key written into api_key (never the secret).
type PluginKeyPolicyKey struct {
	ID         string
	Name       string
	Enabled    bool
	KeyPreview string
}

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
		ID         int64
		APIKey     string
		DisplayKey string
		IsDeleted  bool
	}
	// 先在独立 reader 上比较完整 key 集合，只有变化才进入 writer 事务。
	if err := db.Clauses(dbresolver.Read).Model(&entities.CPAAPIKey{}).
		Where("source_type = ?", entities.CPAAPIKeySourceNative).
		Select("id, api_key, display_key, is_deleted").Find(&existingRows).Error; err != nil {
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
			fields := make(map[string]any)
			if display := helper.RedactSensitiveValue(key); row.DisplayKey != display {
				fields["display_key"] = display
			}
			if row.IsDeleted {
				fields["is_deleted"] = false
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
			SourceType:   entities.CPAAPIKeySourceNative,
			DisplayKey:   helper.RedactSensitiveValue(key),
			IsDeleted:    false,
			LastSyncedAt: &syncedAt,
		})
	}
	staleIDs := make([]int64, 0)
	for _, row := range existingRows {
		if row.IsDeleted {
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
			if err := tx.Model(&entities.CPAAPIKey{}).Where("id = ? AND source_type = ?", change.id, entities.CPAAPIKeySourceNative).Updates(change.fields).Error; err != nil {
				return err
			}
		}
		if len(toCreate) > 0 {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "api_key"}}, DoNothing: true}).CreateInBatches(&toCreate, insertBatchSize(entities.CPAAPIKey{})).Error; err != nil {
				return err
			}
		}
		if len(staleIDs) == 0 {
			return nil
		}
		return tx.Model(&entities.CPAAPIKey{}).Where("id IN ? AND source_type = ?", staleIDs, entities.CPAAPIKeySourceNative).Updates(map[string]any{"is_deleted": true, "updated_at": syncedAt}).Error
	})
}

// SyncPluginKeyPolicyKeys upserts key-policy catalog rows and soft-deletes only
// key-policy rows missing from this ready list. Soft-deleted rows are revived in
// place so local int64 ids stay stable; native rows and other plugin identities
// sharing the unique api_key are never taken over (first writer wins).
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

	var existingRows []entities.CPAAPIKey
	// Read the complete table before taking the writer so unchanged snapshots can return immediately.
	if err := db.Clauses(dbresolver.Read).Model(&entities.CPAAPIKey{}).
		Select("id, api_key, source_type, access_provider, key_alias, display_key, external_id, enabled, is_deleted, last_synced_at").
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
			if !existing.IsKeyPolicyCatalog() {
				// first-writer-wins on uniq api_key; never flip a native or other plugin row to key-policy.
				logrus.WithFields(logrus.Fields{"api_key": key.ID, "source_type": existing.SourceType, "access_provider": existing.AccessProvider}).Error("plugin key-policy sync skipped row owned by another source")
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
			APIKey:         key.ID,
			DisplayKey:     displayKey,
			KeyAlias:       defaultAlias,
			SourceType:     entities.CPAAPIKeySourcePlugin,
			AccessProvider: entities.CPAAPIKeyAccessProviderKeyPolicy,
			ExternalID:     key.ID,
			Enabled:        key.Enabled,
			IsDeleted:      false,
			LastSyncedAt:   &syncedAt,
		})
	}
	// Stale scan only manages exact active plugin rows.
	staleIDs := make([]int64, 0)
	for _, row := range existingRows {
		if row.IsDeleted || !row.IsKeyPolicyCatalog() {
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

// SoftDeleteAllPluginKeyPolicyKeys marks every active key-policy catalog row deleted.
// Used when the plugin endpoint is absent (404/501).
func SoftDeleteAllPluginKeyPolicyKeys(db *gorm.DB, syncedAt time.Time) error {
	return db.Model(&entities.CPAAPIKey{}).
		Where("source_type = ? AND access_provider = ? AND is_deleted = ?", entities.CPAAPIKeySourcePlugin, entities.CPAAPIKeyAccessProviderKeyPolicy, false).
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

func FindActiveCPAAPIKeyByValue(db *gorm.DB, apiKey string) (entities.CPAAPIKey, error) {
	var row entities.CPAAPIKey
	// 该入口用于 API key 登录，插件的统计关联标识不能作为原生凭证。
	err := db.Where("api_key = ? AND is_deleted = ? AND source_type = ?", apiKey, false, entities.CPAAPIKeySourceNative).First(&row).Error
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
