package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONSettingsStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mtools-settings-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	settingsFile := filepath.Join(tempDir, "settings.json")
	fields := []SettingField{
		{
			Key:     "path",
			Type:    SettingFile,
			Default: "/usr/local/bin/yt-dlp",
		},
		{
			Key:     "quality",
			Type:    SettingDropdown,
			Default: "1080",
		},
		{
			Key:     "embed_subtitles",
			Type:    SettingToggle,
			Default: true,
		},
		{
			Key:     "count",
			Type:    SettingNumber,
			Default: 5,
		},
	}

	// 1. Initial store with defaults
	store, err := NewJSONSettingsStore(settingsFile, fields)
	if err != nil {
		t.Fatalf("unexpected error creating store: %v", err)
	}

	if got := store.GetString("path"); got != "/usr/local/bin/yt-dlp" {
		t.Errorf("expected default path, got %q", got)
	}
	if got := store.GetString("quality"); got != "1080" {
		t.Errorf("expected default quality, got %q", got)
	}
	if got := store.GetBool("embed_subtitles"); !got {
		t.Errorf("expected default bool true, got %v", got)
	}
	if got := store.GetInt("count"); got != 5 {
		t.Errorf("expected default count 5, got %v", got)
	}

	// 2. Mutate and Save
	store.Set("path", "/opt/homebrew/bin/yt-dlp")
	store.Set("embed_subtitles", false)
	store.Set("count", 10)

	if err := store.Save(); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// 3. Reload from file
	reloaded, err := NewJSONSettingsStore(settingsFile, fields)
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	if got := reloaded.GetString("path"); got != "/opt/homebrew/bin/yt-dlp" {
		t.Errorf("expected saved path, got %q", got)
	}
	if got := reloaded.GetBool("embed_subtitles"); got {
		t.Errorf("expected saved bool false, got %v", got)
	}
	if got := reloaded.GetInt("count"); got != 10 {
		t.Errorf("expected saved count 10, got %v", got)
	}
	// Untouched key still returns default
	if got := reloaded.GetString("quality"); got != "1080" {
		t.Errorf("expected default quality 1080, got %q", got)
	}
}
