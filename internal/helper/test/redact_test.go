package test

import (
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
)

func TestRedactSensitiveValueUsesCanonicalFormat(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"sk-BabcdefghijklmnopqrstuvwxyzmaWyTA", "sk-*********maWyTA"},
		{"short", "*********"},
		{"sk-123456", "*********"},
		{"", "unknown"},
		{"unknown", "unknown"},
	} {
		t.Run(test.input, func(t *testing.T) {
			if got := helper.RedactSensitiveValue(test.input); got != test.want {
				t.Fatalf("masked value = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCPAAPIKeyDisplayNamePrefersAlias(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-alpha123456", KeyAlias: "  Production  ", DisplayKey: "sk-B********************************Zejy"}

	if got := helper.CPAAPIKeyDisplayName(row); got != "Production" {
		t.Fatalf("expected alias label, got %q", got)
	}
}

func TestCPAAPIKeyDisplayNameFallsBackToMaskedRawKey(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-alpha123456", DisplayKey: "sk-B********************************Zejy"}

	if got := helper.CPAAPIKeyDisplayName(row); got != "sk-*********123456" {
		t.Fatalf("expected canonical masked key fallback, got %q", got)
	}
}

func TestCPAAPIKeyMaskedDisplayKeyMasksRawKeyWithCanonicalFormat(t *testing.T) {
	row := entities.CPAAPIKey{APIKey: "sk-BabcdefghijklmnopqrstuvwxyzmaWyTA", DisplayKey: "sk-B********************************maWy"}

	if got := helper.CPAAPIKeyMaskedDisplayKey(row); got != "sk-*********maWyTA" {
		t.Fatalf("expected canonical display key, got %q", got)
	}
}

func TestCPAAPIKeyMaskedDisplayKeyFallsBackToStoredDisplayKeyWhenRawKeyIsMissing(t *testing.T) {
	row := entities.CPAAPIKey{DisplayKey: "sk-*********maWyTA"}

	if got := helper.CPAAPIKeyMaskedDisplayKey(row); got != "sk-*********maWyTA" {
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
	if got := helper.CPAAPIKeyDisplayName(row); got != "team-a" {
		t.Fatalf("expected plugin id label without redact, got %q", got)
	}
	row.KeyAlias = "Team A"
	if got := helper.CPAAPIKeyDisplayName(row); got != "Team A" {
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
	if got := helper.CPAAPIKeyPublicDisplayName(plugin); got != "plugin-key-7" {
		t.Fatalf("expected stable public plugin fallback, got %q", got)
	}
	plugin.KeyAlias = "Team"
	if got := helper.CPAAPIKeyPublicDisplayName(plugin); got != "Team" {
		t.Fatalf("expected alias, got %q", got)
	}
	native := entities.CPAAPIKey{ID: 3, APIKey: "sk-alpha123456", Source: entities.CPAAPIKeySourceNative}
	if got := helper.CPAAPIKeyPublicDisplayName(native); strings.Contains(got, "sk-alpha123456") {
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
	if got := helper.CPAAPIKeyMaskedDisplayKey(row); got != "cpa_Ab…xy12" {
		t.Fatalf("expected key_preview display, got %q", got)
	}
	row.DisplayKey = ""
	if got := helper.CPAAPIKeyMaskedDisplayKey(row); got != "LiuSha" {
		t.Fatalf("expected logical id fallback without *********, got %q", got)
	}
	// Contrast: native short values still fully mask.
	if got := helper.CPAAPIKeyMaskedDisplayKey(entities.CPAAPIKey{APIKey: "shortkey1", Source: entities.CPAAPIKeySourceNative}); got != "*********" {
		t.Fatalf("expected native short secret mask, got %q", got)
	}
}
