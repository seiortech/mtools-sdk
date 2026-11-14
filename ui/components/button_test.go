package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCircularButtonClick(t *testing.T) {
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		CircularButton(c, "Browse", func() {
			clicked = true
		})
	}, 200, 200)

	tt.Frame()
	if err := tt.Click("Browse"); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	tt.Frame()

	if !clicked {
		t.Fatalf("expected click handler to be invoked")
	}
}

func TestPrimaryButtonClick(t *testing.T) {
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		PrimaryButton(c, "Action", func() {
			clicked = true
		})
	}, 200, 200)

	tt.Frame()
	if err := tt.Click("Action"); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	tt.Frame()

	if !clicked {
		t.Fatalf("expected click handler to be invoked")
	}
}

func TestSecondaryButtonClick(t *testing.T) {
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		SecondaryButton(c, "Cancel", func() {
			clicked = true
		})
	}, 200, 200)

	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	tt.Frame()

	if !clicked {
		t.Fatalf("expected click handler to be invoked")
	}
}

func TestBackButtonClick(t *testing.T) {
	clicked := false
	tt := ui.NewTester(func(c *ui.Context) {
		BackButton(c, func() {
			clicked = true
		})
	}, 200, 200)

	tt.Frame()
	if err := tt.Click("← Back"); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	tt.Frame()

	if !clicked {
		t.Fatalf("expected click handler to be invoked")
	}
}

func TestButtonsAndControlsHeightAlignment(t *testing.T) {
	val := "test value"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			Field(c, FieldProps{
				Label: "Input Label",
				Value: &val,
			})
			PrimaryButton(c, "Primary Action", func() {})
			SecondaryButton(c, "Secondary Action", func() {})
		})
	}, 600, 200)

	tt.Frame()

	pri, okPri := tt.Access("Primary Action")
	if !okPri {
		t.Fatalf("Primary Action not found in access tree")
	}
	sec, okSec := tt.Access("Secondary Action")
	if !okSec {
		t.Fatalf("Secondary Action not found in access tree")
	}

	if pri.Bounds.H != 36 {
		t.Errorf("expected PrimaryButton height 36 DIP, got %f", pri.Bounds.H)
	}
	if sec.Bounds.H != 36 {
		t.Errorf("expected SecondaryButton height 36 DIP, got %f", sec.Bounds.H)
	}
}
