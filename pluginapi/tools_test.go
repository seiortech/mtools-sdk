package pluginapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeDummyFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary-payload"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSharedToolsProviderInterface(t *testing.T) {
	root := t.TempDir()
	sharedDir := filepath.Join(root, "tools")
	pluginsRoot := filepath.Join(root, "plugin-data")
	ownData := filepath.Join(pluginsRoot, "com.mtools.pdf")
	siblingData := filepath.Join(pluginsRoot, "com.mtools.pdfsign")
	legacyDir := filepath.Join(root, "shared", "pdfium")

	settings := map[string]string{
		SettingSharedToolsDir:  sharedDir,
		SettingPluginDataDir:   ownData,
		SettingPluginsDataRoot: pluginsRoot,
	}

	var p SharedToolsProvider = NewSharedTools(settings)
	if got := p.SharedToolsDir(); got != sharedDir {
		t.Fatalf("SharedToolsDir() = %q, want %q", got, sharedDir)
	}

	wantWasmPath := filepath.Join(sharedDir, "pdfium.wasm")
	if got := p.SharedToolPath("pdfium.wasm", false); got != wantWasmPath {
		t.Fatalf("SharedToolPath(pdfium.wasm) = %q, want %q", got, wantWasmPath)
	}

	wantCache := filepath.Join(sharedDir, ".cache", "pdfium")
	if got := p.SharedCacheDir("pdfium", ownData); got != wantCache {
		t.Fatalf("SharedCacheDir() = %q, want %q", got, wantCache)
	}

	// Invalid filenames / traversal rejected
	if got := p.SharedToolPath("../evil", false); got != "" {
		t.Fatalf("expected empty path for traversal filename, got %q", got)
	}
	if got := p.SharedCacheDir("../evil", ownData); got != "" {
		t.Fatalf("expected empty cache dir for traversal name, got %q", got)
	}

	// Nothing installed yet -> not found
	spec := SharedToolSpec{
		FileName:   "pdfium.wasm",
		LegacyDirs: []string{legacyDir},
	}
	if _, ok := p.Resolve("", spec); ok {
		t.Fatal("expected Resolve to return false when nothing exists")
	}

	// 5. Sibling plugin auto-detect
	siblingWasm := filepath.Join(siblingData, "pdfium.wasm")
	writeDummyFile(t, siblingWasm)
	if got, ok := p.Resolve("", spec); !ok || got != siblingWasm {
		t.Fatalf("expected sibling auto-detect %q, got %q (ok=%v)", siblingWasm, got, ok)
	}

	// 4. Legacy dir beats sibling
	legacyWasm := filepath.Join(legacyDir, "pdfium.wasm")
	writeDummyFile(t, legacyWasm)
	if got, ok := p.Resolve("", spec); !ok || got != legacyWasm {
		t.Fatalf("expected legacy dir %q, got %q (ok=%v)", legacyWasm, got, ok)
	}

	// 3. Own plugin data dir beats legacy
	ownWasm := filepath.Join(ownData, "pdfium.wasm")
	writeDummyFile(t, ownWasm)
	if got, ok := p.Resolve("", spec); !ok || got != ownWasm {
		t.Fatalf("expected own plugin data %q, got %q (ok=%v)", ownWasm, got, ok)
	}

	// 2. Shared tools dir beats own plugin data
	writeDummyFile(t, wantWasmPath)
	if got, ok := p.Resolve("", spec); !ok || got != wantWasmPath {
		t.Fatalf("expected shared tools dir %q, got %q (ok=%v)", wantWasmPath, got, ok)
	}

	// 1. Explicit override beats everything
	explicitWasm := filepath.Join(root, "custom", "my-pdfium.wasm")
	writeDummyFile(t, explicitWasm)
	if got, ok := p.Resolve(explicitWasm, spec); !ok || got != explicitWasm {
		t.Fatalf("expected explicit path %q, got %q (ok=%v)", explicitWasm, got, ok)
	}
}

func TestSharedToolsEnvFallbackAndPATH(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "env-tools")
	t.Setenv(EnvSharedToolsDir, envDir)

	st := newSharedToolsWithLookPath(nil, func(file string) (string, error) {
		if file == "yt-dlp" || file == "yt-dlp.exe" {
			return "/usr/local/bin/" + file, nil
		}
		return "", errors.New("not found")
	})
	if st.SharedToolsDir() != envDir {
		t.Fatalf("SharedToolsDir() = %q, want %q", st.SharedToolsDir(), envDir)
	}

	got, ok := st.Resolve("", SharedToolSpec{
		FileName:     "yt-dlp",
		IsExecutable: true,
		CheckPATH:    true,
	})
	want := "/usr/local/bin/yt-dlp"
	if runtime.GOOS == "windows" {
		want = "/usr/local/bin/yt-dlp.exe"
	}
	if !ok || got != want {
		t.Fatalf("PATH fallback = %q (ok=%v), want %q", got, ok, want)
	}

	// Params convenience methods
	rp := RunParams{Settings: map[string]string{SettingSharedToolsDir: envDir}}
	if rp.SharedToolsDir() != envDir {
		t.Fatalf("RunParams.SharedToolsDir() = %q, want %q", rp.SharedToolsDir(), envDir)
	}
	ip := InspectParams{Settings: map[string]string{SettingSharedToolsDir: envDir}}
	if ip.SharedToolsDir() != envDir {
		t.Fatalf("InspectParams.SharedToolsDir() = %q, want %q", ip.SharedToolsDir(), envDir)
	}
	op := UIOpenParams{Settings: map[string]string{SettingSharedToolsDir: envDir}}
	if op.SharedToolsDir() != envDir {
		t.Fatalf("UIOpenParams.SharedToolsDir() = %q, want %q", op.SharedToolsDir(), envDir)
	}
}

func TestSharedToolsIgnoresEmptyFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	sharedDir := filepath.Join(root, "tools")
	// Directory named pdfium.wasm must not match
	if err := os.MkdirAll(filepath.Join(sharedDir, "pdfium.wasm"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Empty 0-byte file must not match
	emptyFile := filepath.Join(root, "empty.wasm")
	if err := os.WriteFile(emptyFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewSharedTools(map[string]string{SettingSharedToolsDir: sharedDir})
	if got, ok := p.Resolve(emptyFile, SharedToolSpec{FileName: "pdfium.wasm"}); ok {
		t.Fatalf("expected false for empty file and directory, got %q", got)
	}
}
