package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Lang represents a language code (e.g. "en", "id", "ja").
type Lang string

const (
	LangEN Lang = "en"
	LangID Lang = "id"
)

// LocaleMeta describes a language JSON file for Settings display and attribution.
type LocaleMeta struct {
	Code   Lang   `json:"code"`
	Name   string `json:"name"`
	Author string `json:"author"`
}

// LocaleFile is the on-disk/embedded JSON format for a single language.
type LocaleFile struct {
	Meta    LocaleMeta        `json:"meta"`
	Strings map[string]string `json:"strings"`
}

// Translator is the standard interface for resolving localized strings
// and inspecting available languages for any scope (app shell or plugin).
type Translator interface {
	T(lang Lang, key string) string
	Tf(lang Lang, key string, args ...any) string
	Available() []LocaleMeta
}

// NormalizeLang returns a normalized Lang code from an arbitrary string or OS locale.
func NormalizeLang(raw string) Lang {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return LangEN
	}
	if strings.HasPrefix(raw, "id") || strings.Contains(raw, "indonesia") {
		return LangID
	}
	if strings.HasPrefix(raw, "en") || strings.Contains(raw, "english") {
		return LangEN
	}
	// Strip region/encoding suffixes (e.g. "ja_JP.UTF-8" -> "ja", "fr-FR" -> "fr").
	if idx := strings.IndexAny(raw, "_-.@"); idx > 0 {
		raw = raw[:idx]
	}
	return Lang(raw)
}

// Catalog holds JSON-loaded translations for one scope (app shell or a single plugin).
type Catalog struct {
	mu      sync.RWMutex
	locales map[Lang]LocaleFile
}

// NewCatalog creates an empty translation catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		locales: make(map[Lang]LocaleFile),
	}
}

// NewCatalogFromFS loads all *.json locale files from dir inside fsys.
func NewCatalogFromFS(fsys fs.FS, dir string) (*Catalog, error) {
	cat := NewCatalog()
	if err := cat.LoadFS(fsys, dir); err != nil {
		return nil, err
	}
	return cat, nil
}

// LoadFS reads all *.json files from dir inside fsys and registers them.
func (c *Catalog) LoadFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := entry.Name()
		if dir != "" && dir != "." {
			path = dir + "/" + entry.Name()
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if err := c.LoadBytes(entry.Name(), data); err != nil {
			return fmt.Errorf("load locale %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// LoadDir reads all *.json files from a filesystem directory if it exists.
// Missing directories are ignored without error.
func (c *Catalog) LoadDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if err := c.LoadBytes(entry.Name(), data); err != nil {
			return fmt.Errorf("load locale %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// LoadBytes parses a single JSON locale file and merges it into the catalog.
func (c *Catalog) LoadBytes(filename string, data []byte) error {
	var lf LocaleFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return err
	}
	code := lf.Meta.Code
	if code == "" {
		base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		code = NormalizeLang(base)
	} else {
		code = NormalizeLang(string(code))
	}
	if code == "" {
		return errors.New("locale code is required")
	}
	lf.Meta.Code = code
	if strings.TrimSpace(lf.Meta.Name) == "" {
		lf.Meta.Name = strings.ToUpper(string(code))
	}
	if lf.Strings == nil {
		lf.Strings = make(map[string]string)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.locales[code]; ok {
		if lf.Meta.Name != "" {
			existing.Meta.Name = lf.Meta.Name
		}
		if lf.Meta.Author != "" {
			existing.Meta.Author = lf.Meta.Author
		}
		for k, v := range lf.Strings {
			existing.Strings[k] = v
		}
		c.locales[code] = existing
	} else {
		c.locales[code] = lf
	}
	return nil
}

// Available returns metadata for all loaded languages in deterministic order.
func (c *Catalog) Available() []LocaleMeta {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]LocaleMeta, 0, len(c.locales))
	for _, lf := range c.locales {
		out = append(out, lf.Meta)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == LangEN {
			return true
		}
		if out[j].Code == LangEN {
			return false
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// DisplayName returns the display name for lang, or uppercased code if unknown.
func (c *Catalog) DisplayName(lang Lang) string {
	norm := NormalizeLang(string(lang))
	c.mu.RLock()
	defer c.mu.RUnlock()
	if lf, ok := c.locales[norm]; ok && lf.Meta.Name != "" {
		return lf.Meta.Name
	}
	return strings.ToUpper(string(norm))
}

// T resolves key for lang, falling back to LangEN, then to key itself.
func (c *Catalog) T(lang Lang, key string) string {
	norm := NormalizeLang(string(lang))
	c.mu.RLock()
	defer c.mu.RUnlock()

	if lf, ok := c.locales[norm]; ok {
		if val, exists := lf.Strings[key]; exists && val != "" {
			return val
		}
	}
	if norm != LangEN {
		if lf, ok := c.locales[LangEN]; ok {
			if val, exists := lf.Strings[key]; exists && val != "" {
				return val
			}
		}
	}
	return key
}

// Tf formats the translated string for lang and key.
func (c *Catalog) Tf(lang Lang, key string, args ...any) string {
	return fmt.Sprintf(c.T(lang, key), args...)
}
