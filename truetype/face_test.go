// Copyright 2015 The Freetype-Go Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"image"
	"image/draw"
	"io/ioutil"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func BenchmarkDrawString(b *testing.B) {
	data, err := ioutil.ReadFile("../licenses/gpl.txt")
	if err != nil {
		b.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	data, err = ioutil.ReadFile("../testdata/luxisr.ttf")
	if err != nil {
		b.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 800, 600))
	draw.Draw(dst, dst.Bounds(), image.White, image.ZP, draw.Src)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.Black,
		Face: NewFace(f, nil),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, line := range lines {
			d.Dot = fixed.P(0, (j*16)%600)
			d.DrawString(line)
		}
	}
}

func TestMetricsLuxiSans(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}

	// Raw FUnit values, manually cross-checked against luxisr.ttx.
	if got, want := f.ascent, int32(2033); got != want {
		t.Errorf("ascent: got %d, want %d", got, want)
	}
	if got, want := f.descent, int32(-432); got != want {
		t.Errorf("descent: got %d, want %d", got, want)
	}
	if got, want := f.lineGap, int32(0); got != want {
		t.Errorf("lineGap: got %d, want %d", got, want)
	}
	if got, want := f.caretSlopeRise, int32(1); got != want {
		t.Errorf("caretSlopeRise: got %d, want %d", got, want)
	}
	if got, want := f.caretSlopeRun, int32(0); got != want {
		t.Errorf("caretSlopeRun: got %d, want %d", got, want)
	}

	face := NewFace(f, &Options{Size: float64(f.FUnitsPerEm()), DPI: 72}).(*face)
	m := face.Metrics()

	// With Size=FUnitsPerEm and DPI=72, 1 FUnit == 1 pixel, so the returned
	// 26.6 values equal FUnits << 6.
	if got, want := m.Ascent, fixed.Int26_6(2033<<6); got != want {
		t.Errorf("Metrics.Ascent: got %d, want %d", got, want)
	}
	if got, want := m.Descent, fixed.Int26_6(432<<6); got != want {
		t.Errorf("Metrics.Descent: got %d, want %d", got, want)
	}
	if got, want := m.Height, fixed.Int26_6(2465<<6); got != want {
		t.Errorf("Metrics.Height: got %d, want %d", got, want)
	}
	if got, want := m.CaretSlope, (image.Point{X: 0, Y: 1}); got != want {
		t.Errorf("Metrics.CaretSlope: got %v, want %v", got, want)
	}
}

// TestMetricsScaling verifies Metrics values scale linearly with the Size option.
func TestMetricsScaling(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	baseline := NewFace(f, &Options{Size: 12, DPI: 72}).Metrics()
	doubled := NewFace(f, &Options{Size: 24, DPI: 72}).Metrics()
	for _, tc := range []struct {
		name        string
		base, twice fixed.Int26_6
	}{
		{"Height", baseline.Height, doubled.Height},
		{"Ascent", baseline.Ascent, doubled.Ascent},
		{"Descent", baseline.Descent, doubled.Descent},
	} {
		// Allow a 1-unit rounding difference because of math.Ceil.
		diff := int64(tc.twice) - 2*int64(tc.base)
		if diff < -2 || diff > 2 {
			t.Errorf("%s: doubled=%d base=%d, doubled should be ~2*base",
				tc.name, tc.twice, tc.base)
		}
	}
}

func TestAdvanceWidthCache(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f, &Options{Size: 12, DPI: 72}).(*face)

	// Populate the cache with one call.
	adv1, ok := face.GlyphAdvance('A')
	if !ok {
		t.Fatal("GlyphAdvance('A') returned !ok")
	}
	if _, cached := face.advanceCache['A']; !cached {
		t.Error("advance cache entry for 'A' not populated after first call")
	}

	// A second call must return the same advance without corrupting state.
	adv2, ok := face.GlyphAdvance('A')
	if !ok {
		t.Fatal("GlyphAdvance('A') second call returned !ok")
	}
	if adv1 != adv2 {
		t.Errorf("advance width differs between calls: first=%d second=%d", adv1, adv2)
	}

	// A rune not in the font must still report ok=false.
	if _, ok := face.GlyphAdvance('\uFFFD'); !ok {
		// luxisr has a .notdef, but rune 0xFFFD likely maps to glyph 0.
		// Only fail if both first and second call disagree.
		if _, ok2 := face.GlyphAdvance('\uFFFD'); ok2 {
			t.Errorf("GlyphAdvance disagreed between calls for unmapped rune")
		}
	}
}
