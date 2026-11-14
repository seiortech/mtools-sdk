package theme

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Mode represents the appearance mode of the application.
type Mode string

const (
	ModeSystem Mode = "system"
	ModeLight  Mode = "light"
	ModeDark   Mode = "dark"
)

// NormalizeMode returns a valid Mode from an arbitrary string.
func NormalizeMode(raw string) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light":
		return ModeLight
	case "dark":
		return ModeDark
	default:
		return ModeSystem
	}
}

var (
	// Sage Garden source palette.
	DeepSage = ui.Hex("#4A5D45")
	MidSage  = ui.Hex("#6B8064")
	Sage     = ui.Hex("#8FA588")
	PaleSage = ui.Hex("#B5C7AE")
	MistSage = ui.Hex("#DCE6D5")

	// Fixed status colors and decorative tones.
	Danger   = ui.Hex("#7A2E2E")
	Warning  = ui.Hex("#7A5A1E")
	Success  = ui.Hex("#2F5D3A")
	OnStatus = ui.Hex("#F7F9F3")

	// Dynamic semantic tokens (updated when theme changes).
	Ground         = MistSage
	Paper          = ui.Hex("#F7F9F3")
	Ink            = ui.Hex("#1F2822")
	InkMuted       = DeepSage
	Accent         = DeepSage
	AccentHover    = ui.Hex("#3E513B")
	AccentPressed  = ui.Hex("#33422F")
	OnField        = ui.Hex("#F7F9F3")
	SurfaceHover   = ui.Hex("#E8EEE5")
	SurfacePressed = MistSage
	FieldSupport   = PaleSage
	Selection      = ui.RGBA(74, 93, 69, 0.18)
	Scrollbar      = ui.RGBA(31, 40, 34, 0.35)
	Quiet          = Sage
	BorderQuiet    = Sage
	Field          = MidSage

	mu          sync.RWMutex
	currentMode = ModeSystem
	isDarkTheme = false
)

func setDarkTokens(dark bool) {
	if dark {
		Ground = ui.Hex("#1F2822")
		Paper = ui.Hex("#33422F")
		Ink = ui.Hex("#F7F9F3")
		InkMuted = PaleSage
		Accent = PaleSage
		AccentHover = MistSage
		AccentPressed = Sage
		OnField = ui.Hex("#1F2822")
		SurfaceHover = ui.Hex("#3E513B")
		SurfacePressed = ui.Hex("#1F2822")
		FieldSupport = ui.Hex("#3E513B")
		Selection = ui.RGBA(181, 199, 174, 0.25)
		Scrollbar = ui.RGBA(247, 249, 243, 0.35)
		Quiet = MidSage
		BorderQuiet = MidSage
		Field = Sage
	} else {
		Ground = MistSage
		Paper = ui.Hex("#F7F9F3")
		Ink = ui.Hex("#1F2822")
		InkMuted = DeepSage
		Accent = DeepSage
		AccentHover = ui.Hex("#3E513B")
		AccentPressed = ui.Hex("#33422F")
		OnField = ui.Hex("#F7F9F3")
		SurfaceHover = ui.Hex("#E8EEE5")
		SurfacePressed = MistSage
		FieldSupport = PaleSage
		Selection = ui.RGBA(74, 93, 69, 0.18)
		Scrollbar = ui.RGBA(31, 40, 34, 0.35)
		Quiet = Sage
		BorderQuiet = Sage
		Field = MidSage
	}
}

// SetMode sets the configured theme mode.
func SetMode(mode Mode) {
	mu.Lock()
	defer mu.Unlock()
	currentMode = NormalizeMode(string(mode))
}

// CurrentMode returns the configured theme mode.
func CurrentMode() Mode {
	mu.RLock()
	defer mu.RUnlock()
	return currentMode
}

// IsDark reports whether the active applied theme is dark.
func IsDark() bool {
	mu.RLock()
	defer mu.RUnlock()
	return isDarkTheme
}

// Apply resolves the effective theme mode against the context's host environment
// and updates both the UI theme and semantic variables.
func Apply(c *ui.Context, mode Mode) bool {
	mode = NormalizeMode(string(mode))
	dark := false
	switch mode {
	case ModeDark:
		dark = true
	case ModeLight:
		dark = false
	case ModeSystem:
		if c != nil && c.Theme() != nil {
			dark = c.Theme().Dark
		}
	}

	mu.Lock()
	currentMode = mode
	isDarkTheme = dark
	setDarkTokens(dark)
	mu.Unlock()

	if c != nil {
		c.SetTheme(SageThemeFor(dark))
	}
	return dark
}

// SageThemeFor returns a Bauhaus x Sage Garden theme for the requested brightness.
func SageThemeFor(dark bool) *ui.Theme {
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
	} else {
		t = ui.LightTheme()
	}
	t.Background = Ground
	t.Surface = Paper
	t.SurfaceHover = SurfaceHover
	t.SurfacePressed = SurfacePressed
	t.Border = Ink
	t.Text = Ink
	t.TextMuted = InkMuted
	t.Accent = Accent
	t.AccentHover = AccentHover
	t.AccentPressed = AccentPressed
	t.AccentText = OnField
	t.InactiveAccent = InkMuted
	t.Danger = Danger
	t.Warning = Warning
	t.Success = Success
	t.Selection = Selection
	t.Focus = Ink
	t.FocusRingWidth = 3
	t.FocusRingOffset = 2
	t.Inverse = Ink
	t.InverseText = Paper
	t.Scrollbar = Scrollbar
	t.Radius = 0
	t.Spacing = 4
	t.FontSize = 15
	t.Font = "Bauhaus Host, system-ui, -apple-system, Segoe UI, sans-serif"
	return t
}

// SageTheme returns the default Bauhaus x Sage Garden design system theme.
func SageTheme() *ui.Theme {
	return SageThemeFor(IsDark())
}
