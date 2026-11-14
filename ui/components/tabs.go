package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// Tabs renders Bauhaus block tabs using ui.TabsBase with Sage Garden styling.
func Tabs(c *ui.Context, key string, selected *int, labels ...string) {
	tabs := ui.TabsBase(c.Key(key), selected, len(labels))

	tabs.List.Wrap().Gap(0).Border(2, theme.Ink).Children(func() {
		for i, label := range labels {
			tab := tabs.Tab(i).Padding(8, 16).Radius(0)
			if i == *selected {
				tab.Background(theme.Accent).
					TextColor(theme.OnField).
					BorderWidth(0, 0, 4, 0).BorderColor(theme.Ink)
			} else {
				tab.Background(theme.Paper).TextColor(theme.Ink)
			}
			tab.Children(func() {
				ui.Text(c, label).FontSize(14).FontWeight(600).SingleLine()
			})
		}
	})
}
