package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestProgressBar(t *testing.T) {
	for _, pct := range []float64{-10, 0, 50, 100, 150} {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ProgressBar(c, pct, 0)
				ProgressBar(c, pct, 8)
			})
		}, 400, 120)
		tt.Frame()
	}
}
