package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// CircularButton renders the Bauhaus circular primary action (88–104 DIP diameter).
func CircularButton(c *ui.Context, label string, onClick func()) ui.Element {
	b := ui.ButtonBase(c).
		Size(96, 96).
		Radius(999).
		Border(4, theme.Ink).
		Justify(ui.Center).
		AlignItems(ui.Center).
		Label(label).
		Cursor(ui.CursorPointer)

	bg := theme.Accent
	var shadowOff float32 = 6

	if b.Pressed() {
		bg = theme.AccentPressed
		shadowOff = 2
	} else if b.Hovered() {
		bg = theme.AccentHover
		shadowOff = 4
	}

	b.Background(bg).
		Shadow(shadowOff, shadowOff, 0, 0, theme.Ink).
		Children(func() {
			ui.Text(c, label).
				FontSize(13).
				FontWeight(700).
				TextColor(theme.OnField).
				SingleLine()
		})

	if b.Clicked() && onClick != nil {
		onClick()
	}

	return b
}

// PrimaryButton renders a standard rectangular Bauhaus button with hard offset shadow.
func PrimaryButton(c *ui.Context, label string, onClick func()) ui.Element {
	b := ui.ButtonBase(c).
		Height(36).
		Padding(0, 18).
		Radius(0).
		Border(2, theme.Ink).
		Justify(ui.Center).
		AlignItems(ui.Center).
		Label(label).
		Cursor(ui.CursorPointer)

	bg := theme.Accent
	var shadowOff float32 = 3

	if b.Pressed() {
		bg = theme.AccentPressed
		shadowOff = 1
	} else if b.Hovered() {
		bg = theme.AccentHover
	}

	b.Background(bg).
		Shadow(shadowOff, shadowOff, 0, 0, theme.Ink).
		Children(func() {
			ui.Text(c, label).
				FontSize(14).
				FontWeight(600).
				TextColor(theme.OnField).
				SingleLine()
		})

	if b.Clicked() && onClick != nil {
		onClick()
	}

	return b
}

// SecondaryButton renders a rectangular Bauhaus button with Paper fill and Ink border.
func SecondaryButton(c *ui.Context, label string, onClick func()) ui.Element {
	b := ui.ButtonBase(c).
		Height(36).
		Padding(0, 14).
		Radius(0).
		Border(2, theme.Ink).
		Justify(ui.Center).
		AlignItems(ui.Center).
		Label(label).
		Cursor(ui.CursorPointer)

	bg := theme.Paper
	var shadowOff float32 = 2

	if b.Pressed() {
		bg = theme.SurfacePressed
		shadowOff = 0
	} else if b.Hovered() {
		bg = theme.SurfaceHover
	}

	b.Background(bg).
		Shadow(shadowOff, shadowOff, 0, 0, theme.Ink).
		Children(func() {
			ui.Text(c, label).
				FontSize(13).
				FontWeight(600).
				TextColor(theme.Ink).
				SingleLine()
		})

	if b.Clicked() && onClick != nil {
		onClick()
	}

	return b
}

// BackButton renders a standard navigation back move button ("← Back").
func BackButton(c *ui.Context, onClick func()) ui.Element {
	return BackButtonWithLabel(c, "← Back", onClick)
}

// BackButtonWithLabel renders a navigation back button with a localized label.
func BackButtonWithLabel(c *ui.Context, label string, onClick func()) ui.Element {
	if label == "" {
		label = "← Back"
	}
	return SecondaryButton(c, label, onClick)
}
