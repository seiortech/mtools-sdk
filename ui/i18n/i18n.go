package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"sync"

	"github.com/egoist/mygo/ui"
	basei18n "github.com/seiortech/mtools-sdk/i18n"
)

//go:embed locales/*.json
var defaultLocalesFS embed.FS

// Lang represents a language code (e.g. "en", "id", "ja").
type Lang = basei18n.Lang

const (
	LangEN Lang = basei18n.LangEN
	LangID Lang = basei18n.LangID
)

// LocaleMeta describes a language JSON file for Settings display and attribution.
type LocaleMeta = basei18n.LocaleMeta

// LocaleFile is the on-disk/embedded JSON format for a single language.
type LocaleFile = basei18n.LocaleFile

// Translator is the standard interface for resolving localized strings
// and inspecting available languages for any scope (app shell or plugin).
type Translator = basei18n.Translator

// Catalog holds JSON-loaded translations for one scope (app shell or a single plugin).
type Catalog = basei18n.Catalog

// NormalizeLang returns a normalized Lang code from an arbitrary string or OS locale.
func NormalizeLang(raw string) Lang {
	return basei18n.NormalizeLang(raw)
}

// NewCatalog creates an empty translation catalog.
func NewCatalog() *Catalog {
	return basei18n.NewCatalog()
}

// NewCatalogFromFS loads all *.json locale files from dir inside fsys.
func NewCatalogFromFS(fsys fs.FS, dir string) (*Catalog, error) {
	return basei18n.NewCatalogFromFS(fsys, dir)
}

var (
	mu            sync.RWMutex
	currentLang   = LangEN
	globalCatalog = mustLoadDefaultGlobal()
)

func mustLoadDefaultGlobal() *Catalog {
	cat, err := basei18n.NewCatalogFromFS(defaultLocalesFS, "locales")
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded i18n locales: %v", err))
	}
	return cat
}

// Global returns the shared application-shell Translator.
func Global() Translator {
	return globalCatalog
}

// InitGlobal loads optional user-provided locale JSON files from userDir into the global catalog.
func InitGlobal(userDir string) error {
	return globalCatalog.LoadDir(userDir)
}

// AvailableLanguages returns metadata for all languages in the global catalog.
func AvailableLanguages() []LocaleMeta {
	return globalCatalog.Available()
}

// DisplayName returns the display name for a language in the global catalog.
func DisplayName(lang Lang) string {
	return globalCatalog.DisplayName(lang)
}

// SetLang explicitly sets the current global language.
func SetLang(lang Lang) {
	mu.Lock()
	defer mu.Unlock()
	currentLang = NormalizeLang(string(lang))
}

// CurrentLang returns the current active global language.
func CurrentLang() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}

// Apply updates the active language for the context and returns the effective language.
func Apply(c *ui.Context, configured Lang) Lang {
	lang := configured
	if lang == "" {
		if c != nil && c.Locale() != "" {
			lang = NormalizeLang(c.Locale())
		} else {
			lang = LangEN
		}
	} else {
		lang = NormalizeLang(string(configured))
	}

	mu.Lock()
	currentLang = lang
	mu.Unlock()
	return lang
}

// T returns the translated string for a given key using the active global language.
func T(key string) string {
	return globalCatalog.T(CurrentLang(), key)
}

// Tf returns the formatted translated string for a given key using the active global language.
func Tf(key string, args ...any) string {
	return globalCatalog.Tf(CurrentLang(), key, args...)
}

// Tr returns either the en or id string directly based on current language.
func Tr(en, id string) string {
	if CurrentLang() == LangID {
		return id
	}
	return en
}
