package i18n

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCatalogCompleteness(t *testing.T) {
	globalCatalog.mu.RLock()
	en := globalCatalog.locales[LangEN]
	id := globalCatalog.locales[LangID]
	globalCatalog.mu.RUnlock()

	if len(en.Strings) == 0 {
		t.Fatal("expected non-empty English locale strings")
	}
	for key, enVal := range en.Strings {
		if enVal == "" {
			t.Errorf("global catalog key %q has empty English translation", key)
		}
		if idVal, ok := id.Strings[key]; !ok || idVal == "" {
			t.Errorf("global catalog key %q missing Indonesian translation", key)
		}
	}
	if en.Meta.Name == "" || en.Meta.Author == "" {
		t.Errorf("expected English meta name and author, got %+v", en.Meta)
	}
	if id.Meta.Name == "" || id.Meta.Author == "" {
		t.Errorf("expected Indonesian meta name and author, got %+v", id.Meta)
	}
}

func TestCatalogTranslatorAndDynamicJSON(t *testing.T) {
	dir := t.TempDir()
	jaJSON := `{
		"meta": {
			"code": "ja",
			"name": "日本語",
			"author": "Community Translator"
		},
		"strings": {
			"greeting": "こんにちは %s"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "ja.json"), []byte(jaJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cat := NewCatalog()
	var tr Translator = cat
	if err := cat.LoadBytes("en.json", []byte(`{"meta":{"code":"en","name":"English","author":"Core"},"strings":{"greeting":"Hello %s","only_en":"English only"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := cat.LoadDir(dir); err != nil {
		t.Fatal(err)
	}

	avail := tr.Available()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available languages, got %d", len(avail))
	}
	if avail[0].Code != LangEN || avail[1].Code != "ja" || avail[1].Name != "日本語" || avail[1].Author != "Community Translator" {
		t.Fatalf("unexpected available metadata: %+v", avail)
	}

	if got := tr.Tf("ja", "greeting", "mtools"); got != "こんにちは mtools" {
		t.Fatalf("expected Japanese greeting, got %q", got)
	}
	// Fallback to English when key missing in Japanese
	if got := tr.T("ja", "only_en"); got != "English only" {
		t.Fatalf("expected English fallback, got %q", got)
	}
	// Fallback to key when missing everywhere
	if got := tr.T("ja", "unknown.key"); got != "unknown.key" {
		t.Fatalf("expected key fallback, got %q", got)
	}
}

func TestLanguageResolution(t *testing.T) {
	SetLang(LangEN)
	if CurrentLang() != LangEN {
		t.Fatalf("expected LangEN, got %v", CurrentLang())
	}
	if T("nav.tool_library") != "Tool Library" {
		t.Fatalf("expected 'Tool Library' in EN, got %q", T("nav.tool_library"))
	}

	SetLang(LangID)
	if CurrentLang() != LangID {
		t.Fatalf("expected LangID, got %v", CurrentLang())
	}
	if T("nav.tool_library") != "Pustaka Alat" {
		t.Fatalf("expected 'Pustaka Alat' in ID, got %q", T("nav.tool_library"))
	}

	// Test Tf
	msg := Tf("library.title", 5)
	if msg != "Alat Terpasang (5)" {
		t.Fatalf("expected 'Alat Terpasang (5)', got %q", msg)
	}

	// Test Tr
	if Tr("Hello", "Halo") != "Halo" {
		t.Fatalf("expected 'Halo' in ID")
	}

	SetLang(LangEN)
	if Tr("Hello", "Halo") != "Hello" {
		t.Fatalf("expected 'Hello' in EN")
	}

	// Test Apply with Tester
	tt := ui.NewTester(func(c *ui.Context) {
		Apply(c, "")
		ui.Text(c, T("nav.history"))
	}, 400, 300)
	tt.SetLocale("id_ID")
	tt.Frame()
	if CurrentLang() != LangID {
		t.Fatalf("expected LangID inferred from locale id_ID, got %v", CurrentLang())
	}
	if !tt.HasText("Riwayat") {
		t.Fatalf("expected 'Riwayat' rendered with id_ID locale")
	}

	// Reset to English
	SetLang(LangEN)
}
