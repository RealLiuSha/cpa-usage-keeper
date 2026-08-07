package service

import (
	"fmt"
	"net/http"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// syncKeyPolicyKeys applies a ready/absent/error decision for the key-policy catalog.
//
// ready (err==nil, including empty keys): SyncPluginKeyPolicyKeys
// absent (404/501): SoftDeleteAllPluginKeyPolicyKeys
// error (other, including network / status 0): do not touch plugin rows
func syncKeyPolicyKeys(db *gorm.DB, result *response.KeyPolicyKeysResult, fetchErr error, now time.Time) error {
	if isKeyPolicyKeysAbsent(result, fetchErr) {
		if db == nil {
			return fmt.Errorf("database is nil")
		}
		logrus.WithField("status_code", keyPolicyStatusCode(result)).Debug("key-policy keys absent; soft-deleting local plugin rows")
		if err := repository.SoftDeleteAllPluginKeyPolicyKeys(db, now); err != nil {
			return fmt.Errorf("soft-delete plugin key-policy keys: %w", err)
		}
		return nil
	}
	if fetchErr != nil {
		// Hard failure: keep last successful plugin snapshot (H6).
		return fmt.Errorf("fetch key-policy keys: %w", fetchErr)
	}
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if result == nil {
		return fmt.Errorf("fetch key-policy keys: empty response")
	}

	keys := make([]repository.PluginKeyPolicyKey, 0, len(result.Payload.Keys))
	for _, item := range result.Payload.Keys {
		keys = append(keys, repository.PluginKeyPolicyKey{
			ID:         item.ID,
			Name:       item.Name,
			Enabled:    item.EffectiveEnabled(),
			KeyPreview: item.KeyPreview,
		})
	}
	if err := repository.SyncPluginKeyPolicyKeys(db, keys, now); err != nil {
		return fmt.Errorf("sync plugin key-policy keys: %w", err)
	}
	return nil
}

func isKeyPolicyKeysAbsent(result *response.KeyPolicyKeysResult, fetchErr error) bool {
	if fetchErr == nil {
		return false
	}
	switch keyPolicyStatusCode(result) {
	case http.StatusNotFound, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

func keyPolicyStatusCode(result *response.KeyPolicyKeysResult) int {
	if result == nil {
		return 0
	}
	return result.StatusCode
}
