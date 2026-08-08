package helper

import (
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
)

const sensitiveValueMask = "*********"

// RedactSensitiveValue 使用统一格式隐藏前端展示中的敏感值：长值保留前 3 位和后 6 位，短值全隐藏。
func RedactSensitiveValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "unknown" {
		return "unknown"
	}
	runes := []rune(trimmed)
	if len(runes) <= 9 {
		return sensitiveValueMask
	}
	return string(runes[:3]) + sensitiveValueMask + string(runes[len(runes)-6:])
}

// CPAAPIKeyMaskedDisplayKey 返回 CPA API Key 的安全展示 key。
// native：优先基于原始 secret 重新脱敏，避免历史 DisplayKey 格式不一致。
// plugin：api_key 是逻辑 id，不得走 secret 脱敏（短 id 会变成 *********）。
func CPAAPIKeyMaskedDisplayKey(row entities.CPAAPIKey) string {
	if entities.IsPluginKeyPolicySource(row.Source) {
		if preview := strings.TrimSpace(row.DisplayKey); preview != "" {
			return preview
		}
		if externalID := strings.TrimSpace(row.ExternalID); externalID != "" {
			return externalID
		}
		if apiKey := strings.TrimSpace(row.APIKey); apiKey != "" {
			return apiKey
		}
		return "unknown"
	}
	if strings.TrimSpace(row.APIKey) != "" {
		return RedactSensitiveValue(row.APIKey)
	}
	return strings.TrimSpace(row.DisplayKey)
}

// CPAAPIKeyDisplayName 返回 CPA API Key 的前端展示名：优先别名，其次按 source 分支的展示 key。
func CPAAPIKeyDisplayName(row entities.CPAAPIKey) string {
	if strings.TrimSpace(row.KeyAlias) != "" {
		return strings.TrimSpace(row.KeyAlias)
	}
	if entities.IsPluginKeyPolicySource(row.Source) {
		if externalID := strings.TrimSpace(row.ExternalID); externalID != "" {
			return externalID
		}
		if apiKey := strings.TrimSpace(row.APIKey); apiKey != "" {
			return apiKey
		}
	}
	return CPAAPIKeyMaskedDisplayKey(row)
}

// CPAAPIKeyPublicDisplayName is for anonymous/share responses.
// Never returns raw APIKey/secret fields; falls back to stable local id when metadata is incomplete.
func CPAAPIKeyPublicDisplayName(row entities.CPAAPIKey) string {
	if alias := strings.TrimSpace(row.KeyAlias); alias != "" {
		return alias
	}
	if entities.IsPluginKeyPolicySource(row.Source) {
		if externalID := strings.TrimSpace(row.ExternalID); externalID != "" {
			return externalID
		}
		if preview := strings.TrimSpace(row.DisplayKey); preview != "" {
			return preview
		}
		if row.ID > 0 {
			return "plugin-key-" + strconv.FormatInt(row.ID, 10)
		}
		return "plugin-key"
	}
	// Native: only masked secret material.
	if masked := CPAAPIKeyMaskedDisplayKey(row); masked != "" && masked != "unknown" {
		return masked
	}
	if row.ID > 0 {
		return "api-key-" + strconv.FormatInt(row.ID, 10)
	}
	return "unknown"
}
