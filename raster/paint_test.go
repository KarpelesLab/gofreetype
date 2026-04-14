// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"image"
	"image/color"
	"testing"
)

// TestAlphaOverPainter confirms that drawing a single span composes
// "over" the destination alpha value.
func TestAlphaOverPainter(t *testing.T) {
	dst := image.NewAlpha(image.Rect(0, 0, 4, 1))
	// Pre-fill dst with 0x80 so we can see the "over" blend.
	for i := range dst.Pix {
		dst.Pix[i] = 0x80
	}
	p := NewAlphaOverPainter(dst)
	p.Paint([]Span{{Y: 0, X0: 1, X1: 3, Alpha: 0x8000}}, true)
	// Pixels 0 and 3 (outside the span) stay at 0x80.
	if dst.Pix[0] != 0x80 || dst.Pix[3] != 0x80 {
		t.Errorf("edge pixels: got %v,%v, want 0x80 each", dst.Pix[0], dst.Pix[3])
	}
	// Pixels 1 and 2 (inside the span) should be blended higher than 0x80.
	if dst.Pix[1] <= 0x80 || dst.Pix[2] <= 0x80 {
		t.Errorf("interior pixels: got %v,%v, want > 0x80", dst.Pix[1], dst.Pix[2])
	}
}

// TestRGBAPainterFill draws a single solid-color span into an RGBA
// image and checks the color.
func TestRGBAPainterFill(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 1))
	p := NewRGBAPainter(dst)
	p.SetColor(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	p.Paint([]Span{{Y: 0, X0: 0, X1: 4, Alpha: 0xffff}}, true)
	for x := 0; x < 4; x++ {
		r, g, b, a := dst.At(x, 0).RGBA()
		if r == 0 || g != 0 || b != 0 || a == 0 {
			t.Errorf("pixel %d: got (%v, %v, %v, %v), want red", x, r, g, b, a)
		}
	}
}

// TestPainterFunc wraps a plain function as a Painter.
func TestPainterFunc(t *testing.T) {
	called := 0
	p := PainterFunc(func(ss []Span, done bool) {
		called += len(ss)
	})
	p.Paint([]Span{{}, {}, {}}, true)
	if called != 3 {
		t.Errorf("PainterFunc called with %d spans, want 3", called)
	}
}

// TestMonochromePainterMergesSpans threads a sequence of adjacent high-
// alpha spans through MonochromePainter and ensures they get merged and
// delivered fully opaque to the wrapped painter.
func TestMonochromePainterMergesSpans(t *testing.T) {
	var received []Span
	inner := PainterFunc(func(ss []Span, done bool) {
		received = append(received, ss...)
	})
	m := NewMonochromePainter(inner)
	m.Paint([]Span{
		{Y: 0, X0: 0, X1: 2, Alpha: 0xffff},
		{Y: 0, X0: 2, X1: 5, Alpha: 0xffff}, // adjacent, should merge
	}, false)
	m.Paint(nil, true)
	// Expect a single span covering 0..5.
	if len(received) != 1 {
		t.Fatalf("received %d spans, want 1: %+v", len(received), received)
	}
	if received[0].X0 != 0 || received[0].X1 != 5 {
		t.Errorf("merged span: got %+v, want X0=0 X1=5", received[0])
	}
}

// TestGammaCorrectionPainter spot-checks that gamma != 1 transforms
// mid-range alpha values, while 1.0 leaves them alone.
func TestGammaCorrectionPainter(t *testing.T) {
	var got []Span
	inner := PainterFunc(func(ss []Span, _ bool) { got = append(got, ss...) })
	p := NewGammaCorrectionPainter(inner, 1.0)
	p.Paint([]Span{{Alpha: 0x8000}}, false)
	if got[0].Alpha != 0x8000 {
		t.Errorf("gamma=1 passthrough: got %#x, want 0x8000", got[0].Alpha)
	}
	got = nil
	p.SetGamma(2.2)
	p.Paint([]Span{{Alpha: 0x8000}}, false)
	if got[0].Alpha == 0x8000 {
		t.Errorf("gamma=2.2: alpha should have changed, got %#x", got[0].Alpha)
	}
}
