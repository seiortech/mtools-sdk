package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// FieldProps configures a styled input field.
type FieldProps struct {
	Label       string
	Description string
	Placeholder string
	Value       *string
	Error       string
	OnChange    func()
}

// Field renders a labeled input with Sage Garden tokens, zero radius, and Bauhaus seams.
func Field(c *ui.Context, props FieldProps) ui.Element {
	var input ui.Element

	ui.Column(c).Gap(6).Children(func() {
		if props.Label != "" {
			ui.Text(c, props.Label).
				FontSize(14).
				FontWeight(600).
				TextColor(theme.Ink)
		}

		if props.Description != "" {
			ui.Text(c, props.Description).
				FontSize(12).
				TextColor(theme.InkMuted)
		}

		input = ui.TextInput(c, props.Value).
			Label(props.Label).
			Placeholder(props.Placeholder).
			Height(36).
			Radius(0).
			Padding(0, 12).
			Border(2, theme.Ink).
			Background(theme.Paper).
			TextColor(theme.Ink).
			FocusRingColor(theme.Ink)

		if props.OnChange != nil {
			input.OnChange(props.OnChange)
		}

		if props.Error != "" {
			input.Invalid(true)
			errMsg := ui.Text(c, props.Error).FontSize(12).TextColor(theme.Danger)
			input.ErrorMessage(errMsg)
		}
	})

	return input
}
