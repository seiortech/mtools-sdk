package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFieldRender(t *testing.T) {
	val := "initial text"
	changed := false

	tt := ui.NewTester(func(c *ui.Context) {
		Field(c, FieldProps{
			Label:       "Target URL",
			Description: "Enter YouTube video link",
			Placeholder: "https://...",
			Value:       &val,
			OnChange: func() {
				changed = true
			},
		})
	}, 400, 200)

	tt.Frame()

	info, ok := tt.AccessByRole(ui.RoleTextField, "Target URL")
	if !ok {
		t.Fatalf("expected Target URL RoleTextField in accessibility tree")
	}
	if info.Value != "initial text" {
		t.Errorf("expected value %q, got %q", "initial text", info.Value)
	}
	if info.Placeholder != "https://..." {
		t.Errorf("expected placeholder %q, got %q", "https://...", info.Placeholder)
	}
	_ = changed
}
