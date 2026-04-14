// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "testing"

// buildAnchorFormat1 builds an Anchor table with (x, y).
func buildAnchorFormat1(x, y int16) []byte {
	var b []byte
	encU16(&b, 1)
	encI16(&b, x)
	encI16(&b, y)
	return b
}

// buildCursive builds a Type-3 Cursive subtable.
func buildCursive(
	coverageGIDs []uint16,
	records []struct {
		entry, exit *Anchor // nil => no anchor
	},
) []byte {
	cov := buildCoverageFormat1(coverageGIDs)
	// header: format + covOff + count = 6 bytes
	// + entryExitRecord table: 4 bytes per record
	// Then coverage.
	// Then anchor tables.
	headerLen := 6 + 4*len(records)
	covOff := headerLen
	// Anchors go after coverage.
	cursor := covOff + len(cov)
	entryOffs := make([]uint16, len(records))
	exitOffs := make([]uint16, len(records))
	var anchors []byte
	for i, r := range records {
		if r.entry != nil {
			entryOffs[i] = uint16(cursor + len(anchors))
			anchors = append(anchors, buildAnchorFormat1(r.entry.X, r.entry.Y)...)
		}
		if r.exit != nil {
			exitOffs[i] = uint16(cursor + len(anchors))
			anchors = append(anchors, buildAnchorFormat1(r.exit.X, r.exit.Y)...)
		}
	}

	var b []byte
	encU16(&b, 1)
	encU16(&b, uint16(covOff))
	encU16(&b, uint16(len(records)))
	for i := range records {
		encU16(&b, entryOffs[i])
		encU16(&b, exitOffs[i])
	}
	b = append(b, cov...)
	b = append(b, anchors...)
	return b
}

func TestGPOSCursive(t *testing.T) {
	sub := buildCursive(
		[]uint16{10, 11, 12},
		[]struct {
			entry, exit *Anchor
		}{
			{entry: &Anchor{X: 0, Y: 100}, exit: &Anchor{X: 500, Y: 100}}, // glyph 10
			{entry: &Anchor{X: 0, Y: 100}, exit: nil},                     // glyph 11 (terminal)
			{entry: nil, exit: &Anchor{X: 500, Y: 100}},                   // glyph 12 (initial)
		},
	)
	data := buildGPOSWithSubtable(3, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	entry, hasEntry, exit, hasExit, ok := tbl.CursiveAnchors(0, 10)
	if !ok {
		t.Fatal("CursiveAnchors(0, 10) ok=false")
	}
	if !hasEntry || entry.X != 0 || entry.Y != 100 {
		t.Errorf("glyph 10 entry: got (%v, %+v), want ok + (0, 100)", hasEntry, entry)
	}
	if !hasExit || exit.X != 500 || exit.Y != 100 {
		t.Errorf("glyph 10 exit: got (%v, %+v), want ok + (500, 100)", hasExit, exit)
	}

	_, _, _, hasExit, ok = tbl.CursiveAnchors(0, 11)
	if !ok {
		t.Fatal("CursiveAnchors(0, 11) ok=false")
	}
	if hasExit {
		t.Error("glyph 11 should have no exit anchor")
	}

	_, hasEntry, _, _, ok = tbl.CursiveAnchors(0, 12)
	if !ok {
		t.Fatal("CursiveAnchors(0, 12) ok=false")
	}
	if hasEntry {
		t.Error("glyph 12 should have no entry anchor")
	}

	if _, _, _, _, ok := tbl.CursiveAnchors(0, 99); ok {
		t.Error("CursiveAnchors(0, 99): ok=true, want false")
	}
}
