package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// ProgressBar renders a bounded Bauhaus progress bar (0..100%).
// When height <= 0, it defaults to 12 DIP.
func ProgressBar(c *ui.Context, pct float64, height float32) {
	if height <= 0 {
		height = 12
	}
	borderW := float32(2)
	if height <= 8 {
		borderW = 1
	}

	var clamped float32
	if pct > 0 {
		if pct > 100 {
			clamped = 100
		} else {
			clamped = float32(pct)
		}
	}

	ui.Box(c).
		Height(height).
		FillWidth().
		Clip().
		Border(borderW, theme.Ink).
		Background(theme.Ground).
		Children(func() {
			if clamped > 0 {
				ui.Box(c).
					FillHeight().
					WidthPercent(clamped).
					Background(theme.Accent)
			}
		})
}
