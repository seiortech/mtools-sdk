package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAsciiParticlePhysics(t *testing.T) {
	p := &AsciiParticle{
		Char:    'M',
		OriginX: 100,
		OriginY: 100,
		X:       100,
		Y:       100,
	}

	// Pointer within repel radius (distance 10 from particle at 100, 100)
	settled := p.Update(100, 90, true, 50, 10)
	if settled {
		t.Fatalf("expected particle to be displaced, not settled")
	}
	if p.Y <= 100 {
		t.Fatalf("expected particle to be repelled downwards (Y > 100), got %v", p.Y)
	}

	// Release touch and step until settled back at origin
	for i := 0; i < 200; i++ {
		settled = p.Update(0, 0, false, 50, 10)
		if settled {
			break
		}
	}

	if !settled {
		t.Fatalf("expected particle to settle back to origin after release")
	}
	if p.X != p.OriginX || p.Y != p.OriginY {
		t.Fatalf("expected particle at origin (%v, %v), got (%v, %v)", p.OriginX, p.OriginY, p.X, p.Y)
	}
}

func TestAsciiLogoRender(t *testing.T) {
	st := &AsciiLogoState{}
	tt := ui.NewTester(func(c *ui.Context) {
		AsciiLogo(c, st)
	}, 800, 300)

	tt.Frame()

	if len(st.Particles) == 0 {
		t.Fatalf("expected particles to be initialized")
	}

	var minX, maxX float32 = 800, 0
	var minY, maxY float32 = 300, 0
	for _, p := range st.Particles {
		if p.OriginX < minX {
			minX = p.OriginX
		}
		if p.OriginX > maxX {
			maxX = p.OriginX
		}
		if p.OriginY < minY {
			minY = p.OriginY
		}
		if p.OriginY > maxY {
			maxY = p.OriginY
		}
	}

	// Verify horizontal and vertical centering
	leftMargin := minX
	rightMargin := float32(800) - (maxX + 8.0)
	if leftMargin < 0 || rightMargin < 0 || (leftMargin-rightMargin > 5.0) || (rightMargin-leftMargin > 5.0) {
		t.Fatalf("expected logo horizontally centered: left=%v, right=%v", leftMargin, rightMargin)
	}

	topMargin := minY
	bottomMargin := float32(140) - (maxY + 13.0) // AsciiLogo height is 140
	if topMargin < 0 || bottomMargin < 0 || (topMargin-bottomMargin > 5.0) || (bottomMargin-topMargin > 5.0) {
		t.Fatalf("expected logo vertically centered in box: top=%v, bottom=%v", topMargin, bottomMargin)
	}
}

func TestAsciiLogoOffsetContainer(t *testing.T) {
	st := &AsciiLogoState{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(100, 50).Children(func() {
			AsciiLogo(c, st)
		})
	}, 800, 600)

	tt.Frame()
	if len(st.Particles) == 0 {
		t.Fatalf("expected particles to be initialized in offset container")
	}
}
