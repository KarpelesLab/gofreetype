// Copyright 2012 The Freetype-Go Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package freetype

import (
	"image"
	"image/draw"
	"os"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/image/math/fixed"
)

func TestDrawStringEndToEnd(t *testing.T) {
	data, err := os.ReadFile("testdata/luxisr.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFont(data)
	if err != nil {
		t.Fatal(err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 400, 80))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)

	c := NewContext()
	c.SetDPI(72)
	c.SetFont(f)
	c.SetFontSize(14)
	c.SetClip(dst.Bounds())
	c.SetDst(dst)
	c.SetSrc(image.Black)
	c.SetHinting(0) // font.HintingNone

	end, err := c.DrawString("Hello, world!", Pt(10, 40))
	if err != nil {
		t.Fatal(err)
	}
	if end.X <= fixed.I(10) {
		t.Errorf("DrawString should advance the pen; got end.X = %v", end.X)
	}
	// Some non-white pixels should appear.
	nonWhite := 0
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			r, g, b, _ := dst.At(x, y).RGBA()
			if r < 0xff00 || g < 0xff00 || b < 0xff00 {
				nonWhite++
			}
		}
	}
	if nonWhite == 0 {
		t.Error("DrawString rendered no non-white pixels")
	}

	// PointToFixed round-trip.
	if got := c.PointToFixed(14); got == 0 {
		t.Errorf("PointToFixed(14) returned zero")
	}
}

func TestContextSetters(t *testing.T) {
	c := NewContext()
	// No-op setters should not panic.
	c.SetDPI(96)
	c.SetDPI(96) // same value — early return path
	c.SetFontSize(10)
	c.SetFontSize(10)
	c.SetHinting(0)
	c.SetHinting(0)
	// DrawString with no font set must error.
	if _, err := c.DrawString("x", Pt(0, 0)); err == nil {
		t.Error("DrawString without a font should error")
	}
}

func TestSetGammaInvalidatesCache(t *testing.T) {
	c := NewContext()
	// Populate a cache entry — any valid one will do; we'll just mutate
	// a slot directly since the high-level DrawString path requires a font.
	c.cache[0] = cacheEntry{valid: true}
	c.SetGamma(2.2)
	if c.cache[0].valid {
		t.Error("SetGamma should invalidate the glyph cache")
	}
	if c.gamma != 2.2 {
		t.Errorf("gamma: got %v, want 2.2", c.gamma)
	}
	// Setting the same gamma again is a no-op.
	c.cache[0] = cacheEntry{valid: true}
	c.SetGamma(2.2)
	if !c.cache[0].valid {
		t.Error("SetGamma to the same value should not invalidate cache")
	}
	// Non-positive gammas clamp to 1.
	c.SetGamma(-1)
	if c.gamma != 1 {
		t.Errorf("gamma after SetGamma(-1): got %v, want 1", c.gamma)
	}
}

func TestScaling(t *testing.T) {
	c := NewContext()
	for _, tc := range [...]struct {
		in   float64
		want fixed.Int26_6
	}{
		{in: 12, want: fixed.I(12)},
		{in: 11.992, want: fixed.I(12) - 1},
		{in: 11.993, want: fixed.I(12)},
		{in: 12.007, want: fixed.I(12)},
		{in: 12.008, want: fixed.I(12) + 1},
		{in: 86.4, want: fixed.Int26_6(86<<6 + 26)},
	} {
		c.SetFontSize(tc.in)
		if got, want := c.scale, tc.want; got != want {
			t.Errorf("scale after SetFontSize(%v) = %v, want %v", tc.in, got, want)
		}
		if got, want := c.PointToFixed(tc.in), tc.want; got != want {
			t.Errorf("PointToFixed(%v) = %v, want %v", tc.in, got, want)
		}
	}
}

func BenchmarkDrawString(b *testing.B) {
	data, err := os.ReadFile("licenses/gpl.txt")
	if err != nil {
		b.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	data, err = os.ReadFile("testdata/luxisr.ttf")
	if err != nil {
		b.Fatal(err)
	}
	f, err := ParseFont(data)
	if err != nil {
		b.Fatal(err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, 800, 600))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)

	c := NewContext()
	c.SetDst(dst)
	c.SetClip(dst.Bounds())
	c.SetSrc(image.Black)
	c.SetFont(f)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	mallocs := ms.Mallocs

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, line := range lines {
			_, err := c.DrawString(line, Pt(0, (j*16)%600))
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	b.StopTimer()
	runtime.ReadMemStats(&ms)
	mallocs = ms.Mallocs - mallocs
	b.Logf("%d iterations, %d mallocs per iteration\n", b.N, int(mallocs)/b.N)
}
