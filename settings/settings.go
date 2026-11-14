package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SettingType identifies the input kind for a setting field.
type SettingType string

const (
	SettingText      SettingType = "text"
	SettingNumber    SettingType = "number"
	SettingToggle    SettingType = "toggle"
	SettingDropdown  SettingType = "dropdown"
	SettingFile      SettingType = "file_picker"
	SettingDirectory SettingType = "directory_picker"
)

// SettingFileFilter defines file dialog filters.
type SettingFileFilter struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
}

// SettingOption defines a selectable choice in a dropdown.
type SettingOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// SettingAction represents a triggerable action attached to a field.
type SettingAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SettingField declares one configurable setting schema entry.
type SettingField struct {
	Key         string             `json:"key"`
	Type        SettingType        `json:"type"`
	Label       string             `json:"label"`
	Description string             `json:"description,omitempty"`
	Placeholder string             `json:"placeholder,omitempty"`
	Default     any                `json:"default,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Filter      *SettingFileFilter `json:"filter,omitempty"`
	Options     []SettingOption    `json:"options,omitempty"`
	Action      *SettingAction     `json:"action,omitempty"`
}

// ActionHandler allows a plugin to handle action button clicks in settings.
type ActionHandler interface {
	RunAction(ctx context.Context, actionID string) error
}

// SettingsStore defines persistence and value access for plugin settings.
type SettingsStore interface {
	Get(key string) any
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	Set(key string, val any)
	Save() error
	GetDefaultPath() string
}

// JSONSettingsStore manages reading and atomic writing of settings to a JSON file.
type JSONSettingsStore struct {
	mu       sync.RWMutex
	filePath string
	values   map[string]any
	defaults map[string]any
}

// NewJSONSettingsStore initializes a store with schema defaults and loads existing data from disk.
func NewJSONSettingsStore(filePath string, fields []SettingField) (*JSONSettingsStore, error) {
	defaults := make(map[string]any, len(fields))
	for _, f := range fields {
		if f.Default != nil {
			defaults[f.Key] = f.Default
		}
	}

	store := &JSONSettingsStore{
		filePath: filePath,
		values:   make(map[string]any),
		defaults: defaults,
	}

	if err := store.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	return store, nil
}

func (s *JSONSettingsStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.values)
}

// Get returns the configured value or default.
func (s *JSONSettingsStore) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.values[key]; ok {
		return val
	}
	return s.defaults[key]
}

// GetString returns setting value as string.
func (s *JSONSettingsStore) GetString(key string) string {
	val := s.Get(key)
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetBool returns setting value as boolean.
func (s *JSONSettingsStore) GetBool(key string) bool {
	val := s.Get(key)
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// GetInt returns setting value as integer.
func (s *JSONSettingsStore) GetInt(key string) int {
	val := s.Get(key)
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

// Set updates a setting value in memory.
func (s *JSONSettingsStore) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = val
}

// Save writes settings atomically to disk.
func (s *JSONSettingsStore) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.values, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create settings dir: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open temp settings file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to write settings: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to sync settings: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to close temp settings file: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to atomically rename settings file: %w", err)
	}

	return nil
}

// GetDefaultPath returns the directory containing the settings file,
// which is the plugin's default data directory.
func (s *JSONSettingsStore) GetDefaultPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filepath.Dir(s.filePath)
}
