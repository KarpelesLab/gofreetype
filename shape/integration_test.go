// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package shape

import (
	"os"
	"testing"

	"github.com/KarpelesLab/gofreetype/layout"
	"github.com/KarpelesLab/gofreetype/truetype"
)

// TestShapeStringLuxiSans is a plain end-to-end shaping run over a font
// that has no GSUB / GPOS tables. Shape still has to turn the runes into
// glyph IDs via cmap, populate advances from hmtx, and successfully
// no-op through the substitution + positioning pipeline.
func TestShapeStringLuxiSans(t *testing.T) {
	data, err := os.ReadFile("../testdata/luxisr.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := truetype.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	opts := Default(layout.MakeTag("latn"), layout.MakeTag("dflt"))
	glyphs := ShapeString(f, "Hello", opts)
	if len(glyphs) != len("Hello") {
		t.Fatalf("glyph count: got %d, want %d", len(glyphs), len("Hello"))
	}
	// 'H' should map to a non-zero glyph.
	if glyphs[0].GID == 0 {
		t.Error("glyph 0 is .notdef — cmap lookup failed")
	}
	// Every glyph should have a positive advance.
	for i, g := range glyphs {
		if g.XAdvance <= 0 {
			t.Errorf("glyph %d XAdvance <= 0: %d", i, g.XAdvance)
		}
	}
	// Clusters should equal the byte offsets passed in (one per rune
	// since all ASCII).
	for i, g := range glyphs {
		if g.Cluster != i {
			t.Errorf("glyph %d cluster: got %d, want %d", i, g.Cluster, i)
		}
	}
}

// TestShapeEmptyInput returns an empty slice without crashing.
func TestShapeEmptyInput(t *testing.T) {
	data, err := os.ReadFile("../testdata/luxisr.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := truetype.Parse(data)
	got := ShapeString(f, "", Default(layout.MakeTag("latn"), layout.MakeTag("dflt")))
	if len(got) != 0 {
		t.Errorf("empty input: got %d glyphs, want 0", len(got))
	}
}

// TestShapeRuneSlice checks the lower-level Shape entry point that
// accepts a rune slice + cluster map directly.
func TestShapeRuneSlice(t *testing.T) {
	data, err := os.ReadFile("../testdata/luxisr.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := truetype.Parse(data)
	runes := []rune{'A', 'B', 'C'}
	clusters := []int{100, 200, 300}
	glyphs := Shape(f, runes, clusters, Default(layout.MakeTag("latn"), layout.MakeTag("dflt")))
	if len(glyphs) != 3 {
		t.Fatalf("got %d glyphs, want 3", len(glyphs))
	}
	// Custom clusters should flow through.
	for i, c := range clusters {
		if glyphs[i].Cluster != c {
			t.Errorf("glyph %d cluster: got %d, want %d", i, glyphs[i].Cluster, c)
		}
	}
}
