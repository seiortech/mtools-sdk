package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVerticalResizeHandleBasic(t *testing.T) {
	currentW := 320
	resetted := false

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Grow(1).FillHeight()
			VerticalResizeHandle(c, ResizeHandleProps{
				Width:        currentW,
				MinWidth:     260,
				MaxWidth:     600,
				DefaultWidth: 320,
				OnResize: func(newW int) {
					currentW = newW
				},
				OnReset: func() {
					resetted = true
				},
			})
			ui.Box(c).Width(float32(currentW)).FillHeight()
		})
	}, 800, 500)

	tt.Frame()

	rect, found := tt.Find("Resize History Panel")
	if !found {
		t.Fatalf("expected 'Resize History Panel' element")
	}

	if rect.W != 6 {
		t.Fatalf("expected handle width 6, got %v", rect.W)
	}

	// Double click to reset
	tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Frame()

	if !resetted {
		t.Fatalf("expected double click to trigger OnReset")
	}
}

func TestVerticalResizeHandleDrag(t *testing.T) {
	currentW := 320

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Grow(1).FillHeight()
			VerticalResizeHandle(c, ResizeHandleProps{
				Width:        currentW,
				MinWidth:     260,
				MaxWidth:     600,
				DefaultWidth: 320,
				OnResize: func(newW int) {
					currentW = newW
				},
			})
			ui.Box(c).Width(float32(currentW)).FillHeight()
		})
	}, 800, 500)

	tt.Frame()

	rect, found := tt.Find("Resize History Panel")
	if !found {
		t.Fatalf("expected 'Resize History Panel' element")
	}

	// Drag to the left by 40 DIP -> width should expand to 360
	tt.Press(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Move(rect.X+rect.W/2-40, rect.Y+rect.H/2)
	tt.Frame()
	tt.Release(rect.X+rect.W/2-40, rect.Y+rect.H/2)
	tt.Frame()

	if currentW != 360 {
		t.Fatalf("expected width to become 360 after drag left by 40, got %d", currentW)
	}

	// Drag to the right by 80 DIP -> width should shrink to 280
	rect, _ = tt.Find("Resize History Panel")
	tt.Press(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Move(rect.X+rect.W/2+80, rect.Y+rect.H/2)
	tt.Frame()
	tt.Release(rect.X+rect.W/2+80, rect.Y+rect.H/2)
	tt.Frame()

	if currentW != 280 {
		t.Fatalf("expected width to become 280 after drag right by 80, got %d", currentW)
	}
}

func TestVerticalResizeHandleClamping(t *testing.T) {
	currentW := 320

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Grow(1).FillHeight()
			VerticalResizeHandle(c, ResizeHandleProps{
				Width:        currentW,
				MinWidth:     260,
				MaxWidth:     600,
				DefaultWidth: 320,
				OnResize: func(newW int) {
					currentW = newW
				},
			})
			ui.Box(c).Width(float32(currentW)).FillHeight()
		})
	}, 800, 500)

	tt.Frame()

	rect, found := tt.Find("Resize History Panel")
	if !found {
		t.Fatalf("expected 'Resize History Panel' element")
	}

	// Drag far to the left (500 DIP) -> clamped at MaxWidth 600
	tt.Press(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Move(rect.X+rect.W/2-500, rect.Y+rect.H/2)
	tt.Frame()
	tt.Release(rect.X+rect.W/2-500, rect.Y+rect.H/2)
	tt.Frame()

	if currentW != 600 {
		t.Fatalf("expected width clamped at 600, got %d", currentW)
	}

	// Drag far to the right (600 DIP) -> clamped at MinWidth 260
	rect, _ = tt.Find("Resize History Panel")
	tt.Press(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Move(rect.X+rect.W/2+600, rect.Y+rect.H/2)
	tt.Frame()
	tt.Release(rect.X+rect.W/2+600, rect.Y+rect.H/2)
	tt.Frame()

	if currentW != 260 {
		t.Fatalf("expected width clamped at 260, got %d", currentW)
	}
}

func TestVerticalResizeHandleKeyboard(t *testing.T) {
	currentW := 320

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Grow(1).FillHeight()
			VerticalResizeHandle(c, ResizeHandleProps{
				Width:        currentW,
				MinWidth:     260,
				MaxWidth:     600,
				DefaultWidth: 320,
				OnResize: func(newW int) {
					currentW = newW
				},
			})
			ui.Box(c).Width(float32(currentW)).FillHeight()
		})
	}, 800, 500)

	tt.Frame()

	rect, found := tt.Find("Resize History Panel")
	if !found {
		t.Fatalf("expected 'Resize History Panel' element")
	}

	// Focus the handle by clicking it
	tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Frame()

	// Press KeyLeft -> expands by 16 DIP
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if currentW != 336 {
		t.Fatalf("expected width 336 after KeyLeft, got %d", currentW)
	}

	// Press KeyRight -> shrinks by 16 DIP
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if currentW != 320 {
		t.Fatalf("expected width 320 after KeyRight, got %d", currentW)
	}
}
