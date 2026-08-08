package helper

import (
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
)

func TestRedactSensitiveValueUsesCanonicalFormat(t *testing.T) {
	if got := RedactSensitiveValue("sk-BabcdefghijklmnopqrstuvwxyzmaWyTA"); got != "sk-*********maWyTA" {
		t.Fatalf("expected canonical masked key, got %q", got)
	}
	if got := RedactSensitiveValue("short"); got != "*********" {
		t.Fatalf("expected short key to use fixed mask, got %q", got)
	}
	if got := RedactSensitiveValue("sk-123456"); got != "*********" {
		t.Fatalf("expected boundary-length key to be fully masked, got %q", got)
	}
	if got := RedactSensitiveValue(""); got != "unknown" {
		t.Fatalf("expected empty key to stay compatible with public fallback, got %q", got)
	}
	if got := RedactSensitiveValue("unknown"); got != "unknown" {
		t.Fatalf("expected unknown key to remain unknown, got %q", got)
	}
}

func TestCPAAPIKeyDisplayNamePrefersAlias(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-alpha123456", KeyAlias: "  Production  ", DisplayKey: "sk-B********************************Zejy"}

	if got := CPAAPIKeyDisplayName(row); got != "Production" {
		t.Fatalf("expected alias label, got %q", got)
	}
}

func TestCPAAPIKeyDisplayNameFallsBackToMaskedRawKey(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-alpha123456", DisplayKey: "sk-B********************************Zejy"}

	if got := CPAAPIKeyDisplayName(row); got != "sk-*********123456" {
		t.Fatalf("expected canonical masked key fallback, got %q", got)
	}
}

func TestCPAAPIKeyMaskedDisplayKeyMasksRawKeyWithCanonicalFormat(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-BabcdefghijklmnopqrstuvwxyzmaWyTA", DisplayKey: "sk-B********************************maWy"}

	if got := CPAAPIKeyMaskedDisplayKey(row); got != "sk-*********maWyTA" {
		t.Fatalf("expected canonical display key, got %q", got)
	}
}

func TestCPAAPIKeyMaskedDisplayKeyFallsBackToStoredDisplayKeyWhenRawKeyIsMissing(t *testing.T) {
	row := entities.CPAAPIKey{DisplayKey: "sk-*********maWyTA"}

	if got := CPAAPIKeyMaskedDisplayKey(row); got != "sk-*********maWyTA" {
		t.Fatalf("expected stored display key fallback, got %q", got)
	}
}

func TestCPAAPIKeyDisplayNameForPluginDoesNotRedactShortID(t *testing.T) {
	row := entities.CPAAPIKey{
		APIKey:     "team-a",
		Source:     entities.CPAAPIKeySourcePluginKeyPolicy,
		ExternalID: "team-a",
		DisplayKey: "cpa_Ab…xy12",
	}
	if got := CPAAPIKeyDisplayName(row); got != "team-a" {
		t.Fatalf("expected plugin id label without redact, got %q", got)
	}
	row.KeyAlias = "Team A"
	if got := CPAAPIKeyDisplayName(row); got != "Team A" {
		t.Fatalf("expected alias, got %q", got)
	}
}

func TestCPAAPIKeyPublicDisplayNameNeverReturnsRawAPIKey(t *testing.T) {
	// Incomplete plugin row: no ExternalID/preview — must not echo APIKey.
	plugin := entities.CPAAPIKey{
		ID:     7,
		APIKey: "secret-or-id",
		Source: entities.CPAAPIKeySourcePluginKeyPolicy,
	}
	if got := CPAAPIKeyPublicDisplayName(plugin); got != "plugin-key-7" {
		t.Fatalf("expected stable public plugin fallback, got %q", got)
	}
	plugin.KeyAlias = "Team"
	if got := CPAAPIKeyPublicDisplayName(plugin); got != "Team" {
		t.Fatalf("expected alias, got %q", got)
	}
	native := entities.CPAAPIKey{ID: 3, APIKey: "sk-alpha123456", Source: entities.CPAAPIKeySourceNative}
	if got := CPAAPIKeyPublicDisplayName(native); strings.Contains(got, "sk-alpha123456") {
		t.Fatalf("public label leaked raw native key: %q", got)
	}
}

func TestCPAAPIKeyMaskedDisplayKeyForPluginUsesPreviewOrID(t *testing.T) {
	row := entities.CPAAPIKey{
		APIKey:     "LiuSha",
		Source:     entities.CPAAPIKeySourcePluginKeyPolicy,
		ExternalID: "LiuSha",
		DisplayKey: "cpa_Ab…xy12",
	}
	if got := CPAAPIKeyMaskedDisplayKey(row); got != "cpa_Ab…xy12" {
		t.Fatalf("expected key_preview display, got %q", got)
	}
	row.DisplayKey = ""
	if got := CPAAPIKeyMaskedDisplayKey(row); got != "LiuSha" {
		t.Fatalf("expected logical id fallback without *********, got %q", got)
	}
	// Contrast: native short values still fully mask.
	if got := CPAAPIKeyMaskedDisplayKey(entities.CPAAPIKey{APIKey: "shortkey1", Source: entities.CPAAPIKeySourceNative}); got != "*********" {
		t.Fatalf("expected native short secret mask, got %q", got)
	}
}
