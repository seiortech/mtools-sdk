package components

import (
	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// ResizeHandleProps configures a vertical resize handle for side panels.
type ResizeHandleProps struct {
	Width        int
	MinWidth     int
	MaxWidth     int
	DefaultWidth int
	OnResize     func(newWidth int)
	OnReset      func()
}

// VerticalResizeHandle renders a draggable splitter divider between workspace and inspector.
func VerticalResizeHandle(c *ui.Context, props ResizeHandleProps) ui.Element {
	minW := props.MinWidth
	if minW <= 0 {
		minW = 260
	}
	maxW := props.MaxWidth
	if maxW <= 0 {
		maxW = 600
	}
	defaultW := props.DefaultWidth
	if defaultW <= 0 {
		defaultW = 320
	}

	handle := ui.Box(c).
		Width(6).
		FillHeight().
		Focusable().
		Role(ui.RoleSplitter).
		Label("Resize History Panel").
		Cursor(ui.CursorResizeEW)

	dx, _, held := handle.Dragged()
	if held && dx != 0 {
		newW := props.Width - int(dx)
		if newW < minW {
			newW = minW
		}
		if newW > maxW {
			newW = maxW
		}
		if newW != props.Width && props.OnResize != nil {
			props.OnResize(newW)
		}
	}

	if handle.DoubleClicked() {
		if props.OnReset != nil {
			props.OnReset()
		} else if props.OnResize != nil {
			props.OnResize(defaultW)
		}
	}

	const step = 16
	if handle.Shortcut(0, ui.KeyLeft) {
		newW := props.Width + step
		if newW > maxW {
			newW = maxW
		}
		if props.OnResize != nil {
			props.OnResize(newW)
		}
	}
	if handle.Shortcut(0, ui.KeyRight) {
		newW := props.Width - step
		if newW < minW {
			newW = minW
		}
		if props.OnResize != nil {
			props.OnResize(newW)
		}
	}

	bg := theme.Ground
	lineColor := theme.Ink
	if held || handle.Hovered() || handle.Focused() {
		bg = theme.FieldSupport
		lineColor = theme.Accent
	}

	return handle.
		Background(bg).
		BorderWidth(0, 1, 0, 0).
		BorderColor(lineColor)
}
