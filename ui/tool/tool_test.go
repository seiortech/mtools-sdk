package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPluginPathsMethods(t *testing.T) {
	tempDir := t.TempDir()
	paths := NewPluginPaths(tempDir)

	if paths.GetDefaultPath() != filepath.Clean(tempDir) {
		t.Fatalf("expected default path %q, got %q", tempDir, paths.GetDefaultPath())
	}

	expectedHist := filepath.Join(tempDir, "history.json")
	if paths.GetHistoryPath() != expectedHist {
		t.Fatalf("expected history path %q, got %q", expectedHist, paths.GetHistoryPath())
	}

	expectedSettings := filepath.Join(tempDir, "settings.json")
	if paths.GetSettingsPath() != expectedSettings {
		t.Fatalf("expected settings path %q, got %q", expectedSettings, paths.GetSettingsPath())
	}

	expectedBin := filepath.Join(tempDir, "yt-dlp")
	if runtime.GOOS == "windows" {
		expectedBin += ".exe"
	}
	if paths.GetBinaryPath("yt-dlp") != expectedBin {
		t.Fatalf("expected binary path %q, got %q", expectedBin, paths.GetBinaryPath("yt-dlp"))
	}

	expectedCache := filepath.Join(tempDir, "cache")
	if paths.GetCacheDir() != expectedCache {
		t.Fatalf("expected cache dir %q, got %q", expectedCache, paths.GetCacheDir())
	}

	expectedTemp := filepath.Join(tempDir, "tmp")
	if paths.GetTempDir() != expectedTemp {
		t.Fatalf("expected temp dir %q, got %q", expectedTemp, paths.GetTempDir())
	}

	// Test EnsureDirs
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}
	if fi, err := os.Stat(expectedCache); err != nil || !fi.IsDir() {
		t.Fatalf("expected cache dir to exist")
	}
	if fi, err := os.Stat(expectedTemp); err != nil || !fi.IsDir() {
		t.Fatalf("expected tmp dir to exist")
	}

	// Test ResolveFile
	subFile, err := paths.ResolveFile("sub", "file.txt")
	if err != nil {
		t.Fatalf("ResolveFile failed: %v", err)
	}
	expectedSubFile := filepath.Join(tempDir, "sub", "file.txt")
	if subFile != expectedSubFile {
		t.Fatalf("expected %q, got %q", expectedSubFile, subFile)
	}

	// Test path escape rejection
	if _, err := paths.ResolveFile("..", "escape.txt"); err == nil {
		t.Fatalf("expected path escape error for '..'")
	}
	if _, err := paths.ResolveFile("sub", "..", "..", "escape.txt"); err == nil {
		t.Fatalf("expected path escape error for nested traversal")
	}

	// Test CleanTemp
	dummyTempFile := filepath.Join(expectedTemp, "scratch.txt")
	if err := os.WriteFile(dummyTempFile, []byte("temp"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := paths.CleanTemp(); err != nil {
		t.Fatalf("CleanTemp failed: %v", err)
	}
	if _, err := os.Stat(dummyTempFile); !os.IsNotExist(err) {
		t.Fatalf("expected dummy temp file to be removed")
	}

	// Test ClearCache
	dummyCacheFile := filepath.Join(expectedCache, "thumb.jpg")
	if err := os.WriteFile(dummyCacheFile, []byte("cache"), 0644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}
	if err := paths.ClearCache(); err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}
	if _, err := os.Stat(dummyCacheFile); !os.IsNotExist(err) {
		t.Fatalf("expected dummy cache file to be removed")
	}
}

func TestDefaultPluginDataDir(t *testing.T) {
	dataDir := DefaultPluginDataDir("/tmp/user", "com.mtools.youtube")
	expected := filepath.Join("/tmp/user", "plugin-data", "com.mtools.youtube")
	if !strings.HasSuffix(dataDir, filepath.FromSlash("plugin-data/com.mtools.youtube")) {
		t.Fatalf("expected %q, got %q", expected, dataDir)
	}
}
