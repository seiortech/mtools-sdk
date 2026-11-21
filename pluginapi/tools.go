package pluginapi

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	// EnvSharedToolsDir is injected by the host runtime when launching sidecars.
	// It points to the user's configured shared managed tools directory
	// (default: <PathUserData>/tools).
	EnvSharedToolsDir = "MTOOLS_SHARED_TOOLS_DIR"

	// Well-known host-forwarded keys carried in UIOpenParams.Settings,
	// RunParams.Settings, and InspectParams.Settings.
	SettingSharedToolsDir  = "shared_tools_dir"
	SettingPluginsDataRoot = "plugins_data_root"
	SettingPluginDataDir   = "plugin_data_dir"
)

// SharedToolSpec describes a managed binary or runtime asset to resolve across
// the shared tools folder, plugin data directories, legacy locations, and PATH.
type SharedToolSpec struct {
	// FileName is the base filename to look for (e.g. "pdfium.wasm", "yt-dlp",
	// "ffmpeg", "u2netp.onnx"). Must be a plain filename without path separators.
	FileName string

	// IsExecutable appends ".exe" on Windows when FileName does not already end
	// with ".exe".
	IsExecutable bool

	// Subdirs lists optional relative subdirectories (e.g. []string{"models"})
	// to check inside SharedToolsDir, plugin_data_dir, and sibling plugin dirs.
	Subdirs []string

	// LegacyDirs lists optional absolute fallback directories (e.g. legacy
	// "<dataDir>/shared/pdfium") checked after plugin_data_dir.
	LegacyDirs []string

	// LegacyPaths lists optional absolute legacy file paths (e.g. an older
	// global "pdfium_wasm_path" entry) checked after plugin_data_dir.
	LegacyPaths []string

	// CheckPATH enables exec.LookPath fallback for CLI executables.
	CheckPATH bool
}

// SharedToolsProvider gives plugin developers a single, testable interface to
// query the host's default shared tools directory and auto-detect shared
// binaries or runtime assets installed by any plugin.
type SharedToolsProvider interface {
	// SharedToolsDir returns the effective default shared tools directory
	// configured by the user in App Settings (default: <PathUserData>/tools).
	SharedToolsDir() string

	// SharedToolPath returns the canonical installation path inside
	// SharedToolsDir for a managed binary or asset (appending ".exe" on Windows
	// when isExecutable is true). Returns "" if SharedToolsDir is empty or
	// fileName is invalid.
	SharedToolPath(fileName string, isExecutable bool) string

	// SharedCacheDir returns "<SharedToolsDir>/.cache/<cacheName>" when
	// SharedToolsDir is set, falling back to "<fallbackDataDir>/<cacheName>"
	// when SharedToolsDir is empty.
	SharedCacheDir(cacheName, fallbackDataDir string) string

	// Resolve locates an existing binary or runtime asset in strict priority order:
	//   1. explicitPath (user override in plugin settings)
	//   2. SharedToolsDir (<dir>/<name>, <dir>/bin/<name>, <dir>/<subdir>/<name>)
	//   3. plugin_data_dir (<data>/<name>, <data>/bin/<name>, <data>/<subdir>/<name>)
	//   4. LegacyPaths and LegacyDirs
	//   5. sibling plugin directories under plugins_data_root/*/{<name>,bin/<name>,<subdir>/<name>}
	//   6. system PATH (when spec.CheckPATH is true)
	Resolve(explicitPath string, spec SharedToolSpec) (string, bool)
}

type sharedTools struct {
	sharedDir       string
	pluginDataDir   string
	pluginsDataRoot string
	lookPath        func(string) (string, error)
}

// NewSharedTools builds a SharedToolsProvider from host-forwarded RPC settings
// (UIOpenParams.Settings, RunParams.Settings, or InspectParams.Settings),
// falling back to $MTOOLS_SHARED_TOOLS_DIR when SettingSharedToolsDir is absent.
func NewSharedTools(settings map[string]string) SharedToolsProvider {
	return newSharedToolsWithLookPath(settings, exec.LookPath)
}

func newSharedToolsWithLookPath(settings map[string]string, lookPath func(string) (string, error)) *sharedTools {
	var sharedDir, pluginDataDir, pluginsDataRoot string
	if settings != nil {
		sharedDir = strings.TrimSpace(settings[SettingSharedToolsDir])
		pluginDataDir = strings.TrimSpace(settings[SettingPluginDataDir])
		pluginsDataRoot = strings.TrimSpace(settings[SettingPluginsDataRoot])
	}
	if sharedDir == "" {
		sharedDir = strings.TrimSpace(os.Getenv(EnvSharedToolsDir))
	}
	if sharedDir != "" {
		sharedDir = filepath.Clean(sharedDir)
	}
	if pluginDataDir != "" {
		pluginDataDir = filepath.Clean(pluginDataDir)
	}
	if pluginsDataRoot != "" {
		pluginsDataRoot = filepath.Clean(pluginsDataRoot)
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &sharedTools{
		sharedDir:       sharedDir,
		pluginDataDir:   pluginDataDir,
		pluginsDataRoot: pluginsDataRoot,
		lookPath:        lookPath,
	}
}

func (t *sharedTools) SharedToolsDir() string {
	if t == nil {
		return ""
	}
	return t.sharedDir
}

func (t *sharedTools) SharedToolPath(fileName string, isExecutable bool) string {
	if t == nil || t.sharedDir == "" {
		return ""
	}
	name, ok := sanitizeToolFileName(fileName, isExecutable)
	if !ok {
		return ""
	}
	return filepath.Join(t.sharedDir, name)
}

func (t *sharedTools) SharedCacheDir(cacheName, fallbackDataDir string) string {
	name, ok := sanitizeSingleSegment(cacheName)
	if !ok {
		return ""
	}
	if t != nil && t.sharedDir != "" {
		return filepath.Join(t.sharedDir, ".cache", name)
	}
	if fb := strings.TrimSpace(fallbackDataDir); fb != "" {
		return filepath.Join(filepath.Clean(fb), name)
	}
	return ""
}

func (t *sharedTools) Resolve(explicitPath string, spec SharedToolSpec) (string, bool) {
	if t == nil {
		return "", false
	}
	// 1. Explicit user override in plugin settings
	if p := strings.TrimSpace(explicitPath); p != "" {
		if isRegularNonEmptyFile(p) {
			return filepath.Clean(p), true
		}
	}

	name, ok := sanitizeToolFileName(spec.FileName, spec.IsExecutable)
	if !ok {
		return "", false
	}

	// Build candidate relative subpaths: root, "bin", plus any validated spec.Subdirs.
	subdirs := make([]string, 0, 2+len(spec.Subdirs))
	subdirs = append(subdirs, "", "bin")
	for _, sd := range spec.Subdirs {
		if clean, valid := sanitizeRelativeSubdir(sd); valid {
			subdirs = append(subdirs, clean)
		}
	}

	checkDir := func(dir string) (string, bool) {
		if dir == "" {
			return "", false
		}
		for _, sub := range subdirs {
			var cand string
			if sub == "" {
				cand = filepath.Join(dir, name)
			} else {
				cand = filepath.Join(dir, sub, name)
			}
			if isRegularNonEmptyFile(cand) {
				return cand, true
			}
		}
		return "", false
	}

	// 2. Shared managed tools directory
	if p, found := checkDir(t.sharedDir); found {
		return p, true
	}

	// 3. Current plugin's own data directory
	if p, found := checkDir(t.pluginDataDir); found {
		return p, true
	}

	// 4. Legacy explicit paths and directories
	for _, lp := range spec.LegacyPaths {
		if p := strings.TrimSpace(lp); p != "" && isRegularNonEmptyFile(p) {
			return filepath.Clean(p), true
		}
	}
	for _, ld := range spec.LegacyDirs {
		if p, found := checkDir(strings.TrimSpace(ld)); found {
			return p, true
		}
	}

	// 5. Sibling plugin directories under plugins_data_root/*
	if t.pluginsDataRoot != "" {
		if entries, err := os.ReadDir(t.pluginsDataRoot); err == nil {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, dirName := range names {
				sibling := filepath.Join(t.pluginsDataRoot, dirName)
				if sibling == t.pluginDataDir {
					continue
				}
				if p, found := checkDir(sibling); found {
					return p, true
				}
			}
		}
	}

	// 6. System PATH fallback for CLI executables
	if spec.CheckPATH && t.lookPath != nil {
		if p, err := t.lookPath(name); err == nil && p != "" {
			return p, true
		}
		if base := strings.TrimSpace(spec.FileName); base != "" && base != name {
			if p, err := t.lookPath(base); err == nil && p != "" {
				return p, true
			}
		}
	}

	return "", false
}

func sanitizeToolFileName(fileName string, isExecutable bool) (string, bool) {
	name, ok := sanitizeSingleSegment(fileName)
	if !ok {
		return "", false
	}
	if isExecutable && runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	return name, true
}

func sanitizeSingleSegment(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return "", false
	}
	if strings.ContainsAny(s, `/\`) || strings.ContainsRune(s, 0) {
		return "", false
	}
	return s, true
}

func sanitizeRelativeSubdir(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || filepath.IsAbs(s) || strings.ContainsRune(s, 0) {
		return "", false
	}
	clean := filepath.Clean(s)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func isRegularNonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	return true
}
