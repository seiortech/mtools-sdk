package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/i18n"
)

// PluginID identifies a plugin by reverse-DNS notation.
type PluginID string

// ToolID identifies a tool within a plugin.
type ToolID string

// ToolMeta describes a user-facing tool.
type ToolMeta struct {
	ID          ToolID   `json:"id"`
	PluginID    PluginID `json:"plugin_id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Icon        string   `json:"icon,omitempty"`
}

// Tool is the base interface implemented by all tools.
type Tool interface {
	Meta() ToolMeta
}

// NativeTool draws its own native view using MyGo.
type NativeTool interface {
	Tool
	View(c *ui.Context)
}

// HistoryEntry is a normalized view of a completed task for global history.
type HistoryEntry struct {
	ID        string    `json:"id"`
	ToolID    ToolID    `json:"tool_id"`
	ToolName  string    `json:"tool_name"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle"`
	FilePath  string    `json:"file_path"`
	Timestamp time.Time `json:"timestamp"`
}

// HistoryProvider is an optional interface tools implement to surface history globally.
type HistoryProvider interface {
	HistoryEntries() []HistoryEntry
	DeleteHistory(id string) error
	ClearHistory() error
}

// LocaleProvider is an optional interface plugins/tools implement to expose their
// per-plugin translation catalog and accept a host language resolver.
type LocaleProvider interface {
	LocaleTranslator() i18n.Translator
	SetLanguageResolver(fn func() i18n.Lang)
}

// WorkspaceLifecycle is an optional interface tools/plugins implement so the host
// can initialize workspace state on open, check for active work, and cancel/release
// resources when the workspace is closed.
type WorkspaceLifecycle interface {
	OnOpen()
	OnClose()
	HasActiveJob() bool
}

// DefaultPluginDataDir returns the standard data folder (<userDataDir>/plugin-data/<pluginID>)
// where settings.json, history.json, and managed binaries live.
func DefaultPluginDataDir(userDataDir string, pluginID PluginID) string {
	return filepath.Join(userDataDir, "plugin-data", string(pluginID))
}

// PluginStorage defines standard directory and file path resolution for a plugin.
// All plugin-owned persistent and temporary files live under GetDefaultPath().
type PluginStorage interface {
	// GetDefaultPath returns the plugin root data directory (same folder as history.json).
	GetDefaultPath() string

	// GetHistoryPath returns the path to the plugin's history file (<defaultPath>/history.json).
	GetHistoryPath() string

	// GetSettingsPath returns the path to the plugin's settings file (<defaultPath>/settings.json).
	GetSettingsPath() string

	// GetBinaryPath returns the default path for a helper executable in the plugin directory.
	GetBinaryPath(binaryName string) string

	// GetCacheDir returns the plugin's disposable cache directory (<defaultPath>/cache).
	GetCacheDir() string

	// GetTempDir returns the plugin's scratch directory (<defaultPath>/tmp).
	GetTempDir() string

	// ResolveFile safely joins path elements within GetDefaultPath(), preventing directory traversal.
	ResolveFile(elem ...string) (string, error)

	// EnsureDirs ensures default, cache, and tmp directories exist.
	EnsureDirs() error

	// CleanTemp removes contents of GetTempDir().
	CleanTemp() error

	// ClearCache removes contents of GetCacheDir().
	ClearCache() error
}

// PluginPaths provides a standard, reusable implementation of PluginStorage.
type PluginPaths struct {
	root string
}

// NewPluginPaths constructs a PluginPaths resolver for a given root data directory.
func NewPluginPaths(root string) PluginPaths {
	return PluginPaths{root: filepath.Clean(root)}
}

// GetDefaultPath returns the root plugin data folder.
func (p PluginPaths) GetDefaultPath() string {
	return p.root
}

// GetHistoryPath returns <root>/history.json.
func (p PluginPaths) GetHistoryPath() string {
	return filepath.Join(p.root, "history.json")
}

// GetSettingsPath returns <root>/settings.json.
func (p PluginPaths) GetSettingsPath() string {
	return filepath.Join(p.root, "settings.json")
}

// GetBinaryPath returns <root>/<binaryName> (appending .exe on Windows if missing).
func (p PluginPaths) GetBinaryPath(binaryName string) string {
	name := binaryName
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	return filepath.Join(p.root, name)
}

// GetCacheDir returns <root>/cache.
func (p PluginPaths) GetCacheDir() string {
	return filepath.Join(p.root, "cache")
}

// GetTempDir returns <root>/tmp.
func (p PluginPaths) GetTempDir() string {
	return filepath.Join(p.root, "tmp")
}

// ErrPathEscape indicates an attempted directory traversal escape.
var ErrPathEscape = errors.New("path escapes plugin data directory")

// ResolveFile joins elem to root and ensures the resulting path is strictly contained within root.
func (p PluginPaths) ResolveFile(elem ...string) (string, error) {
	joined := filepath.Join(append([]string{p.root}, elem...)...)
	cleaned := filepath.Clean(joined)
	rel, err := filepath.Rel(p.root, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("%w: %s", ErrPathEscape, joined)
	}
	return cleaned, nil
}

// EnsureDirs creates root, cache, and tmp directories.
func (p PluginPaths) EnsureDirs() error {
	if p.root == "" {
		return errors.New("empty plugin root directory")
	}
	if err := os.MkdirAll(p.root, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(p.GetCacheDir(), 0755); err != nil {
		return err
	}
	return os.MkdirAll(p.GetTempDir(), 0755)
}

// CleanTemp removes all entries inside <root>/tmp.
func (p PluginPaths) CleanTemp() error {
	tmp := p.GetTempDir()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
	}
	return nil
}

// ClearCache removes all entries inside <root>/cache.
func (p PluginPaths) ClearCache() error {
	cache := p.GetCacheDir()
	entries, err := os.ReadDir(cache)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(cache, e.Name()))
	}
	return nil
}

// DependencyStatus describes a required or optional helper executable.
type DependencyStatus struct {
	ID          string
	Name        string
	Required    bool
	Found       bool
	Path        string
	DefaultPath string
}

// DependencyProvider is an optional interface for plugins requiring external helpers.
type DependencyProvider interface {
	CheckDependencies() []DependencyStatus
	InstallDependencyToDefault(ctx context.Context, depID string, progress func(done float64, msg string)) (string, error)
}


