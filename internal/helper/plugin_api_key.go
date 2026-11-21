package helper

import (
	"encoding/json"
	"strings"

	"cpa-usage-keeper/internal/entities"
)

const pluginAPIKeyPrefix = "plugin:"

// IsPluginAccessProvider 只判断上游插件命名空间，不拆解或改写完整来源。
func IsPluginAccessProvider(provider string) bool {
	return strings.HasPrefix(provider, pluginAPIKeyPrefix) && strings.TrimSpace(strings.TrimPrefix(provider, pluginAPIKeyPrefix)) != ""
}

// IsKeyPolicyAccessProvider matches the cpa-plugin-key-policy namespace. CPA reports it as
// "plugin:<plugin id>:<provider id>", i.e. "plugin:cpa-key-policy:cpa-key-policy".
func IsKeyPolicyAccessProvider(provider string) bool {
	return provider == entities.CPAAPIKeyAccessProviderKeyPolicy ||
		strings.HasPrefix(provider, entities.CPAAPIKeyAccessProviderKeyPolicy+":")
}

// PluginAPIKeyGroupKey 使用固定顺序的紧凑 JSON，避免来源与 Principal 中的分隔符歧义。
func PluginAPIKeyGroupKey(provider, apiKey string) string {
	encoded, _ := json.Marshal([2]string{provider, apiKey}) // 字符串数组没有不可编码的值。
	return pluginAPIKeyPrefix + string(encoded)
}

// IsPluginAPIKeyGroupKey 仅供展示边界识别内部编码，不用于重新编码已存事件。
func IsPluginAPIKeyGroupKey(value string) bool {
	if !strings.HasPrefix(value, pluginAPIKeyPrefix+"[") {
		return false
	}
	var identity []string
	return json.Unmarshal([]byte(strings.TrimPrefix(value, pluginAPIKeyPrefix)), &identity) == nil &&
		len(identity) == 2 && IsPluginAccessProvider(identity[0]) && strings.TrimSpace(identity[1]) != ""
}
