package components

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

var asciiMtools = []string{
	` /$$      /$$ /$$$$$$$$  /$$$$$$   /$$$$$$  /$$        /$$$$$$ `,
	`| $$$    /$$$|__  $$__/ /$$__  $$ /$$__  $$| $$       /$$__  $$`,
	`| $$$$  /$$$$   | $$   | $$  \ $$| $$  \ $$| $$      |  $$$$$$/`,
	`| $$ $$/$$ $$   | $$   | $$  | $$| $$  | $$| $$       \____  $$`,
	`| $$  $$$| $$   | $$   | $$  | $$| $$  | $$| $$       /$$  \ $$`,
	`| $$\  $ | $$   | $$   |  $$$$$$/|  $$$$$$/| $$$$$$$$|  $$$$$$/`,
	`|__/     |__/   |__/    \______/  \______/ |________/ \______/ `,
}

// AsciiParticle is one ASCII character reacting to pointer interactions.
type AsciiParticle struct {
	Char    rune
	OriginX float32
	OriginY float32
	X       float32
	Y       float32
	VX      float32
	VY      float32
}

// Update steps physics: repelling away from touch, spring pulling to origin, and friction dampening.
func (p *AsciiParticle) Update(touchX, touchY float32, touched bool, repelRadius, repelPower float32) bool {
	if touched {
		dx := p.X - touchX
		dy := p.Y - touchY
		distSq := dx*dx + dy*dy
		dist := float32(math.Sqrt(float64(distSq)))

		if dist < repelRadius && dist > 0.001 {
			angle := float32(math.Atan2(float64(dy), float64(dx)))
			force := (repelRadius - dist) / repelRadius
			push := force * repelPower

			p.VX += float32(math.Cos(float64(angle))) * push
			p.VY += float32(math.Sin(float64(angle))) * push
		}
	}

	// Spring back to origin
	p.VX += (p.OriginX - p.X) * 0.08
	p.VY += (p.OriginY - p.Y) * 0.08

	// Friction
	p.VX *= 0.88
	p.VY *= 0.88

	p.X += p.VX
	p.Y += p.VY

	// Snap and settle threshold
	if !touched &&
		float32(math.Abs(float64(p.VX))) < 0.01 &&
		float32(math.Abs(float64(p.VY))) < 0.01 &&
		float32(math.Abs(float64(p.OriginX-p.X))) < 0.1 &&
		float32(math.Abs(float64(p.OriginY-p.Y))) < 0.1 {
		p.X = p.OriginX
		p.Y = p.OriginY
		p.VX = 0
		p.VY = 0
		return true
	}

	return p.X == p.OriginX && p.Y == p.OriginY && p.VX == 0 && p.VY == 0
}

// AsciiLogoState tracks the particles simulation across frames.
type AsciiLogoState struct {
	Particles []*AsciiParticle
	W         float32
	H         float32
}

func (s *AsciiLogoState) init(width, height float32) {
	s.W = width
	s.H = height

	maxCols := 0
	for _, line := range asciiMtools {
		if len(line) > maxCols {
			maxCols = len(line)
		}
	}
	numRows := len(asciiMtools)

	var charSpacingX float32 = 8.0
	var charSpacingY float32 = 16.0
	var fontSize float32 = 13.0

	targetWidth := width * 0.90
	if float32(maxCols)*charSpacingX > targetWidth && targetWidth > 0 {
		scale := targetWidth / (float32(maxCols) * charSpacingX)
		if scale < 0.5 {
			scale = 0.5
		}
		charSpacingX *= scale
		charSpacingY *= scale
		fontSize *= scale
	}

	totalWidth := float32(maxCols) * charSpacingX
	totalHeight := float32(numRows-1)*charSpacingY + fontSize
	startX := (width - totalWidth) / 2
	startY := (height - totalHeight) / 2

	s.Particles = make([]*AsciiParticle, 0, maxCols*numRows)
	for rowIdx, line := range asciiMtools {
		for colIdx, ch := range line {
			if ch != ' ' {
				posX := startX + float32(colIdx)*charSpacingX
				posY := startY + float32(rowIdx)*charSpacingY
				s.Particles = append(s.Particles, &AsciiParticle{
					Char:    ch,
					OriginX: posX,
					OriginY: posY,
					X:       posX,
					Y:       posY,
				})
			}
		}
	}
}

// AsciiLogo renders interactive repelling ASCII art "MTOOLS".
func AsciiLogo(c *ui.Context, st *AsciiLogoState) ui.Element {
	box := ui.Box(c).
		Key("mtools_ascii_logo").
		Height(140).
		FillWidth().
		Cursor(ui.CursorCrosshair)

	reduceMotion := c.Preferences().ReduceMotion
	px, py, over := box.PointerPosition()

	return box.Draw(func(p *ui.Painter, r ui.Rect) {
		if st.Particles == nil || st.W != r.W || st.H != r.H {
			st.init(r.W, r.H)
		}

		repelRadius := float32(80.0)
		repelPower := float32(14.0)

		moving := false
		for _, pt := range st.Particles {
			if !reduceMotion {
				settled := pt.Update(px, py, over, repelRadius, repelPower)
				if !settled {
					moving = true
				}
			}

			// Sage Garden color differentiation
			col := theme.Ink
			if pt.Char == '/' || pt.Char == '\\' || pt.Char == '_' || pt.Char == '|' {
				col = theme.Quiet
			}

			fontSize := float32(13.0)
			if st.W > 0 {
				ratio := (st.W * 0.90) / (64.0 * 8.0)
				if ratio < 1.0 {
					fontSize *= ratio
					if fontSize < 7.0 {
						fontSize = 7.0
					}
				}
			}

			p.Text(r.X+pt.X, r.Y+pt.Y, string(pt.Char), fontSize, col)
		}

		if moving || over {
			p.AnimationFrame()
		}
	})
}
