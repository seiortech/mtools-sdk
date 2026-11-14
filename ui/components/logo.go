package components

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// LogoConfig holds visual options for the mtools logo.
type LogoConfig struct {
	Size        float32
	StrokeColor ui.Color
	Radius      [4]float32
}

// DefaultLogoConfig returns the standard 240 DIP configuration.
func DefaultLogoConfig() LogoConfig {
	return LogoConfig{
		Size:        240,
		StrokeColor: theme.Ink,
		Radius:      [4]float32{16, 12, 8, 4},
	}
}

// HeartbeatScale calculates the pulse scale factor for t in [0, 1].
// ponytail: piece-wise linear interpolation of Compose keyframes; upgrade when physical spring is specified.
func HeartbeatScale(t float32) float32 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	switch {
	case t <= 0.125: // 0 -> 250ms: 1.00 -> 1.08
		return 1.0 + (t/0.125)*0.08
	case t <= 0.250: // 250 -> 500ms: 1.08 -> 1.02
		return 1.08 - ((t-0.125)/0.125)*0.06
	case t <= 0.400: // 500 -> 800ms: 1.02 -> 1.15
		return 1.02 + ((t-0.250)/0.150)*0.13
	case t <= 0.600: // 800 -> 1200ms: 1.15 -> 1.00
		return 1.15 - ((t-0.400)/0.200)*0.15
	default: // 1200 -> 2000ms: 1.00
		return 1.00
	}
}

// Logo renders the animated mtools mark with concentric counter-rotating boxes and heartbeat scaling.
func Logo(c *ui.Context, opts ...LogoConfig) ui.Element {
	cfg := DefaultLogoConfig()
	if len(opts) > 0 {
		cfg = opts[0]
	}

	container := ui.Box(c).Key("mtools_logo").Size(cfg.Size, cfg.Size)

	var rot float32
	var scale float32 = 1.0

	if !c.Preferences().ReduceMotion {
		rot = container.Loop("mtools_logo_rot", 8*time.Second, ui.Linear) * 360
		beat := container.Loop("mtools_logo_beat", 2*time.Second, ui.Linear)
		scale = HeartbeatScale(beat)
	}

	container.Scale(scale, scale)

	baseRatio := cfg.Size / 240.0
	r := cfg.Radius

	// Layer 1: 160 DIP box, 3 DIP border, clockwise rotation
	b1Size := 160 * baseRatio
	b1Offset := (cfg.Size - b1Size) / 2
	b1Border := max(1, 3*baseRatio)

	// Layer 2: 120 DIP box, 2 DIP border, counter-clockwise rotation (45 - rot)
	b2Size := 120 * baseRatio
	b2Offset := (cfg.Size - b2Size) / 2
	b2Border := max(1, 2*baseRatio)

	// Layer 3: 80 DIP box, 2 DIP border, clockwise rotation (rot + 45)
	b3Size := 80 * baseRatio
	b3Offset := (cfg.Size - b3Size) / 2
	b3Border := max(1, 2*baseRatio)

	// Layer 4: 36 DIP center core, solid fill, counter-clockwise rotation (45 - rot)
	b4Size := 36 * baseRatio
	b4Offset := (cfg.Size - b4Size) / 2

	return container.Children(func() {
		ui.Box(c).Key("b1").Absolute().
			Left(b1Offset).Top(b1Offset).
			Size(b1Size, b1Size).
			Rotate(rot).
			Border(b1Border, cfg.StrokeColor).
			Radius(r[0] * baseRatio)

		ui.Box(c).Key("b2").Absolute().
			Left(b2Offset).Top(b2Offset).
			Size(b2Size, b2Size).
			Rotate(45 - rot).
			Border(b2Border, cfg.StrokeColor).
			Radius(r[1] * baseRatio)

		ui.Box(c).Key("b3").Absolute().
			Left(b3Offset).Top(b3Offset).
			Size(b3Size, b3Size).
			Rotate(rot + 45).
			Border(b3Border, cfg.StrokeColor).
			Radius(r[2] * baseRatio)

		ui.Box(c).Key("b4").Absolute().
			Left(b4Offset).Top(b4Offset).
			Size(b4Size, b4Size).
			Rotate(45 - rot).
			Background(cfg.StrokeColor).
			Radius(r[3] * baseRatio)
	})
}
