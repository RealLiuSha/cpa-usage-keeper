package entities

import "time"

// CPA API Key source constants. Native rows are login credentials; plugin rows
// are analysis match keys only and must never authenticate Key Viewer sessions.
const (
	CPAAPIKeySourceNative          = "native"
	CPAAPIKeySourcePluginKeyPolicy = "plugin:cpa-key-policy"
)

// CPAAPIKey 保存 CPA 管理接口同步到本地的 API-Key，完整 key 仅供后端内部查询使用。
//
// api_key 一列三义（遗留约束）：
//   - native：登录凭据（auth）与脱敏源（helper/redact）
//   - plugin：分析匹配键（usage_events.api_group_key JOIN）
//
// 登录侧必须用 native 谓词收紧；展示侧必须按 source 分支。
// 今后任何新增 WHERE api_key = ? 都必须先回答「要的是凭据还是匹配键」。
type CPAAPIKey struct {
	ID         int64  `gorm:"primaryKey"`
	APIKey     string `gorm:"uniqueIndex:uniq_cpa_api_keys_api_key"`
	DisplayKey string
	KeyAlias   string
	// Source defaults at migration/backfill time; sync paths always set it explicitly.
	Source     string `gorm:"index:idx_cpa_api_keys_source_is_deleted,priority:1;default:native"`
	ExternalID string
	// Enabled has no gorm default tag: a default would make GORM skip bool zero values on Create,
	// so disabled plugin catalog keys could not be inserted as enabled=false. Callers must set it.
	Enabled              bool
	LocalRankingAvatarID *uint8
	IsDeleted            bool       `gorm:"index:idx_cpa_api_keys_is_deleted;index:idx_cpa_api_keys_source_is_deleted,priority:2"`
	LastSyncedAt         *time.Time `gorm:"serializer:storageTime"`
	CreatedAt            time.Time  `gorm:"serializer:storageTime"`
	UpdatedAt            time.Time  `gorm:"serializer:storageTime"`
}

// IsNativeCPAAPIKeySource reports whether a source value is managed as native
// (including empty/NULL historical rows that predate the source column).
func IsNativeCPAAPIKeySource(source string) bool {
	return source == "" || source == CPAAPIKeySourceNative
}

// IsPluginKeyPolicySource reports whether a source value is exactly the key-policy plugin source.
func IsPluginKeyPolicySource(source string) bool {
	return source == CPAAPIKeySourcePluginKeyPolicy
}
