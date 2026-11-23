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

	mu               sync.RWMutex
	currentMode      = ModeSystem
	isDarkTheme      = false
	tokensInit       = false
	cachedLightTheme = buildSageTheme(false)
	cachedDarkTheme  = buildSageTheme(true)
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
	if !tokensInit || isDarkTheme != dark {
		isDarkTheme = dark
		tokensInit = true
		setDarkTokens(dark)
	}
	mu.Unlock()

	if c != nil {
		c.SetTheme(cachedSageTheme(dark))
	}
	return dark
}

func cachedSageTheme(dark bool) *ui.Theme {
	if dark {
		return cachedDarkTheme
	}
	return cachedLightTheme
}

func buildSageTheme(dark bool) *ui.Theme {
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
		t.Background = ui.Hex("#1F2822")
		t.Surface = ui.Hex("#33422F")
		t.SurfaceHover = ui.Hex("#3E513B")
		t.SurfacePressed = ui.Hex("#1F2822")
		t.Border = ui.Hex("#F7F9F3")
		t.Text = ui.Hex("#F7F9F3")
		t.TextMuted = PaleSage
		t.Accent = PaleSage
		t.AccentHover = MistSage
		t.AccentPressed = Sage
		t.AccentText = ui.Hex("#1F2822")
		t.InactiveAccent = PaleSage
		t.Selection = ui.RGBA(181, 199, 174, 0.25)
		t.Focus = ui.Hex("#F7F9F3")
		t.Inverse = ui.Hex("#F7F9F3")
		t.InverseText = ui.Hex("#33422F")
		t.Scrollbar = ui.RGBA(247, 249, 243, 0.35)
	} else {
		t = ui.LightTheme()
		t.Background = MistSage
		t.Surface = ui.Hex("#F7F9F3")
		t.SurfaceHover = ui.Hex("#E8EEE5")
		t.SurfacePressed = MistSage
		t.Border = ui.Hex("#1F2822")
		t.Text = ui.Hex("#1F2822")
		t.TextMuted = DeepSage
		t.Accent = DeepSage
		t.AccentHover = ui.Hex("#3E513B")
		t.AccentPressed = ui.Hex("#33422F")
		t.AccentText = ui.Hex("#F7F9F3")
		t.InactiveAccent = DeepSage
		t.Selection = ui.RGBA(74, 93, 69, 0.18)
		t.Focus = ui.Hex("#1F2822")
		t.Inverse = ui.Hex("#1F2822")
		t.InverseText = ui.Hex("#F7F9F3")
		t.Scrollbar = ui.RGBA(31, 40, 34, 0.35)
	}
	t.Danger = Danger
	t.Warning = Warning
	t.Success = Success
	t.FocusRingWidth = 3
	t.FocusRingOffset = 2
	t.Radius = 0
	t.Spacing = 4
	t.FontSize = 15
	t.Font = "Bauhaus Host, system-ui, -apple-system, Segoe UI, sans-serif"
	return t
}

// SageThemeFor returns a Bauhaus x Sage Garden theme copy for the requested brightness.
func SageThemeFor(dark bool) *ui.Theme {
	cp := *cachedSageTheme(dark)
	return &cp
}

// SageTheme returns the default Bauhaus x Sage Garden design system theme.
func SageTheme() *ui.Theme {
	return SageThemeFor(IsDark())
}
