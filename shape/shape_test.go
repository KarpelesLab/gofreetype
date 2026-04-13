// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package shape

import (
	"testing"

	"github.com/KarpelesLab/gofreetype/layout"
)

// TestDefaultFeatureBundle verifies the canonical feature bundle contains
// the core shaping features applied by most client code.
func TestDefaultFeatureBundle(t *testing.T) {
	opts := Default(layout.MakeTag("latn"), layout.MakeTag("dflt"))
	have := make(map[layout.Tag]bool)
	for _, f := range opts.Features {
		have[f] = true
	}
	for _, required := range []string{"liga", "kern", "mark", "mkmk"} {
		if !have[layout.MakeTag(required)] {
			t.Errorf("Default bundle missing required feature %q", required)
		}
	}
}

// TestSpliceGlyphsLengthMismatch exercises the fallback path in
// spliceGlyphs that allocates a new buffer when the replacement length
// differs from the consumed length (e.g. a 2-glyph ligature collapse or
// a 1-glyph decomposition).
func TestSpliceGlyphsLengthMismatch(t *testing.T) {
	buf := []Glyph{
		{GID: 1, Cluster: 0},
		{GID: 2, Cluster: 1},
		{GID: 3, Cluster: 2},
	}
	// Collapse [0..2) into a single glyph.
	out := spliceGlyphs(buf, 0, 2, []uint16{100})
	if len(out) != 2 {
		t.Fatalf("len after collapse: got %d, want 2", len(out))
	}
	if out[0].GID != 100 || out[0].Cluster != 0 {
		t.Errorf("collapsed glyph: got %+v, want {100, 0}", out[0])
	}
	if out[1].GID != 3 || out[1].Cluster != 2 {
		t.Errorf("tail glyph: got %+v, want {3, 2}", out[1])
	}

	// Expand [1..2) into two glyphs.
	buf = []Glyph{
		{GID: 1, Cluster: 0},
		{GID: 2, Cluster: 1},
		{GID: 3, Cluster: 2},
	}
	out = spliceGlyphs(buf, 1, 1, []uint16{200, 201})
	if len(out) != 4 {
		t.Fatalf("len after expand: got %d, want 4", len(out))
	}
	if out[1].GID != 200 || out[2].GID != 201 {
		t.Errorf("expanded glyphs: got GIDs %d,%d want 200,201", out[1].GID, out[2].GID)
	}
	if out[1].Cluster != 1 || out[2].Cluster != 1 {
		t.Errorf("expanded glyphs should share source cluster 1, got %d,%d",
			out[1].Cluster, out[2].Cluster)
	}
}

// TestSpliceGlyphsSameLength ensures the in-place fast path keeps clusters.
func TestSpliceGlyphsSameLength(t *testing.T) {
	buf := []Glyph{
		{GID: 1, Cluster: 0},
		{GID: 2, Cluster: 1},
		{GID: 3, Cluster: 2},
	}
	out := spliceGlyphs(buf, 0, 3, []uint16{10, 20, 30})
	for i, want := range []uint16{10, 20, 30} {
		if out[i].GID != want {
			t.Errorf("out[%d].GID: got %d, want %d", i, out[i].GID, want)
		}
		if out[i].Cluster != i {
			t.Errorf("out[%d].Cluster: got %d, want %d (should be preserved)",
				i, out[i].Cluster, i)
		}
	}
}
