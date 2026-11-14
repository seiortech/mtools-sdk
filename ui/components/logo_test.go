package components

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestHeartbeatScale(t *testing.T) {
	tests := []struct {
		timeFrac float32
		want     float32
	}{
		{0.0, 1.00},
		{0.125, 1.08},
		{0.250, 1.02},
		{0.400, 1.15},
		{0.600, 1.00},
		{0.800, 1.00},
		{1.000, 1.00},
	}

	for _, tc := range tests {
		got := HeartbeatScale(tc.timeFrac)
		if math.Abs(float64(got-tc.want)) > 0.005 {
			t.Errorf("HeartbeatScale(%v) = %v, want %v", tc.timeFrac, got, tc.want)
		}
	}
}

func TestLogoBuild(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		Logo(c)
	}, 400, 400)
	tt.Frame()

	// Advance time and ensure frame renders
	tt.Advance(100 * time.Millisecond)
	tt.Frame()
}

func TestLogoReduceMotion(t *testing.T) {
	tt := ui.NewTesterWith(func(c *ui.Context) {
		Logo(c)
	}, 400, 400, ui.TesterOptions{
		Preferences: ui.Preferences{ReduceMotion: true},
	})
	tt.Frame()

	// When motion is reduced, loop should not schedule continuous animation frames
	if tt.FramePending() {
		t.Fatalf("expected no frame pending when ReduceMotion is true")
	}
}
