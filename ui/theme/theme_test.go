package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestThemeModesAndTokens(t *testing.T) {
	// 1. Light Mode test
	tt := ui.NewTester(func(c *ui.Context) {
		Apply(c, ModeLight)
		ui.Text(c, "Hello Light")
	}, 400, 300)
	tt.Frame()

	if IsDark() {
		t.Fatalf("expected IsDark to be false in ModeLight")
	}
	if Ground != MistSage {
		t.Fatalf("expected Ground to be MistSage in Light mode, got %v", Ground)
	}
	if Paper != ui.Hex("#F7F9F3") {
		t.Fatalf("expected Paper to be #F7F9F3 in Light mode, got %v", Paper)
	}
	if Ink != ui.Hex("#1F2822") {
		t.Fatalf("expected Ink to be #1F2822 in Light mode, got %v", Ink)
	}
	if Accent != DeepSage {
		t.Fatalf("expected Accent to be DeepSage in Light mode, got %v", Accent)
	}
	if OnField != ui.Hex("#F7F9F3") {
		t.Fatalf("expected OnField to be #F7F9F3 in Light mode, got %v", OnField)
	}

	// 2. Dark Mode test
	ttDark := ui.NewTester(func(c *ui.Context) {
		Apply(c, ModeDark)
		ui.Text(c, "Hello Dark")
	}, 400, 300)
	ttDark.Frame()

	if !IsDark() {
		t.Fatalf("expected IsDark to be true in ModeDark")
	}
	if Ground != ui.Hex("#1F2822") {
		t.Fatalf("expected Ground to be #1F2822 in Dark mode, got %v", Ground)
	}
	if Paper != ui.Hex("#33422F") {
		t.Fatalf("expected Paper to be #33422F in Dark mode, got %v", Paper)
	}
	if Ink != ui.Hex("#F7F9F3") {
		t.Fatalf("expected Ink to be #F7F9F3 in Dark mode, got %v", Ink)
	}
	if Accent != PaleSage {
		t.Fatalf("expected Accent to be PaleSage in Dark mode, got %v", Accent)
	}
	if OnField != ui.Hex("#1F2822") {
		t.Fatalf("expected OnField to be #1F2822 in Dark mode, got %v", OnField)
	}

	// 3. System Mode test with tt.SetDark
	ttSys := ui.NewTester(func(c *ui.Context) {
		Apply(c, ModeSystem)
		ui.Text(c, "Hello System")
	}, 400, 300)
	ttSys.SetDark(false)
	ttSys.Frame()
	if IsDark() {
		t.Fatalf("expected IsDark false when tester dark is false")
	}

	ttSys.SetDark(true)
	ttSys.Frame()
	if !IsDark() {
		t.Fatalf("expected IsDark true when tester dark is true")
	}
	if Ground != ui.Hex("#1F2822") {
		t.Fatalf("expected Ground #1F2822 when system dark is active")
	}

	// Restore Light mode for subsequent tests
	Apply(nil, ModeLight)
}
