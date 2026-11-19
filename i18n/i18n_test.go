package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if got := tr.T("ja", "only_en"); got != "English only" {
		t.Fatalf("expected English fallback, got %q", got)
	}
	if got := tr.T("ja", "unknown.key"); got != "unknown.key" {
		t.Fatalf("expected key fallback, got %q", got)
	}
}
