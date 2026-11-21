package entities

import "time"

const (
	CPAAPIKeySourceNative = "native"
	CPAAPIKeySourcePlugin = "plugin"
	// CPAAPIKeyAccessProviderKeyPolicy marks plugin rows synced from the
	// cpa-plugin-key-policy public catalog. Their api_key is the plugin's logical
	// key id, which is also the usage api_group_key, so history stays keyed by it.
	CPAAPIKeyAccessProviderKeyPolicy = "plugin:cpa-key-policy"
)

// PluginAPIKeyIdentity 只交付用量提交后登记需要的统一标识和完整认证来源。
type PluginAPIKeyIdentity struct {
	APIGroupKey    string
	AccessProvider string
}

// CPAAPIKey 保存原生密钥或插件统计身份，内部关联值仅供后端使用。
//
// api_key means different things per source: a login credential for native rows,
// and an analysis match key (usage_events.api_group_key) for plugin rows. Login
// must filter on source_type = native; display must branch on the source.
type CPAAPIKey struct {
	ID             int64  `gorm:"primaryKey"`
	APIKey         string `gorm:"uniqueIndex:uniq_cpa_api_keys_api_key"`
	DisplayKey     string
	KeyAlias       string
	SourceType     string `gorm:"not null;default:'native'"`
	AccessProvider string `gorm:"not null;default:''"`
	// ExternalID and Enabled are only maintained for key-policy catalog rows.
	ExternalID string
	// Enabled has no gorm default tag: a default would make GORM skip bool zero values on Create,
	// so disabled plugin catalog keys could not be inserted as enabled=false.
	Enabled      bool
	IsDeleted    bool       `gorm:"index:idx_cpa_api_keys_is_deleted"`
	LastSyncedAt *time.Time `gorm:"serializer:storageTime"`
	CreatedAt    time.Time  `gorm:"serializer:storageTime"`
	UpdatedAt    time.Time  `gorm:"serializer:storageTime"`
}

// IsKeyPolicyCatalog reports whether the row is owned by the key-policy catalog sync.
func (k CPAAPIKey) IsKeyPolicyCatalog() bool {
	return k.SourceType == CPAAPIKeySourcePlugin && k.AccessProvider == CPAAPIKeyAccessProviderKeyPolicy
}

// IsEnabled reports the effective enabled state: only key-policy catalog rows carry
// a toggle; native keys and discovered plugin identities are always active.
func (k CPAAPIKey) IsEnabled() bool {
	if k.IsKeyPolicyCatalog() {
		return k.Enabled
	}
	return true
}
