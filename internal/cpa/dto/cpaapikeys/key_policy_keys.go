package cpaapikeys

// KeyPolicyPublicKey is the public catalog entry from
// GET /v0/management/plugins/cpa-key-policy/keys.
// Only whitelist fields used by Keeper; plain_key / key_hash are never decoded.
type KeyPolicyPublicKey struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    *bool  `json:"enabled"`
	KeyPreview string `json:"key_preview"`
}

// KeyPolicyKeysResponse is the catalog payload after CPA host unwrapping.
type KeyPolicyKeysResponse struct {
	Keys []KeyPolicyPublicKey `json:"keys"`
}

// EffectiveEnabled returns true when enabled is omitted (plugin default).
func (k KeyPolicyPublicKey) EffectiveEnabled() bool {
	if k.Enabled == nil {
		return true
	}
	return *k.Enabled
}
