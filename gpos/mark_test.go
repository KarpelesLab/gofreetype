// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "testing"

// buildMarkToBase builds a Type-4 mark-to-base subtable.
//
// Layout (from subtable start):
//
//	0: header (12 bytes: format, markCovOff, baseCovOff, markClassCount,
//	    markArrayOff, baseArrayOff)
//	12: MarkArray
//	...: BaseArray
//	...: mark coverage
//	...: base coverage
//	...: Anchor tables inline in MarkArray/BaseArray regions
//
// For simplicity, we place mark and base coverages at the tail.
func buildMarkToBase(
	markGIDs []uint16,
	baseGIDs []uint16,
	markClassCount int,
	// markRecords: per mark (in markGIDs order) = (markClass, anchor)
	markRecords []struct {
		class  uint16
		anchor Anchor
	},
	// baseAnchors: per base (in baseGIDs order) = [markClassCount] anchors
	// (nil Anchor pointer = no anchor for that class).
	baseAnchors [][]*Anchor,
) []byte {
	// Compose MarkArray bytes.
	// Layout: u16 markCount + [class u16, anchorOff u16]*markCount + anchor tables.
	markArray := make([]byte, 2+4*len(markRecords))
	markArray[0] = byte(len(markRecords) >> 8)
	markArray[1] = byte(len(markRecords))
	for i, r := range markRecords {
		markArray[2+4*i] = byte(r.class >> 8)
		markArray[2+4*i+1] = byte(r.class)
		// anchor offset filled in below
	}
	for i, r := range markRecords {
		// Anchor offset relative to markArray start.
		aOff := uint16(len(markArray))
		markArray[2+4*i+2] = byte(aOff >> 8)
		markArray[2+4*i+3] = byte(aOff)
		markArray = append(markArray, buildAnchorFormat1(r.anchor.X, r.anchor.Y)...)
	}

	// Compose BaseArray bytes.
	// Layout: u16 baseCount + [markClassCount * anchorOff u16]*baseCount +
	//   anchor tables (offsets relative to BaseArray start).
	baseArray := make([]byte, 2+2*markClassCount*len(baseAnchors))
	baseArray[0] = byte(len(baseAnchors) >> 8)
	baseArray[1] = byte(len(baseAnchors))
	for b, row := range baseAnchors {
		for c := 0; c < markClassCount; c++ {
			if c < len(row) && row[c] != nil {
				aOff := uint16(len(baseArray))
				base := 2 + (b*markClassCount+c)*2
				baseArray[base] = byte(aOff >> 8)
				baseArray[base+1] = byte(aOff)
				baseArray = append(baseArray, buildAnchorFormat1(row[c].X, row[c].Y)...)
			}
			// else: leave 0 offset (no anchor).
		}
	}

	markCov := buildCoverageFormat1(markGIDs)
	baseCov := buildCoverageFormat1(baseGIDs)

	// Subtable layout: header (12) + MarkArray + BaseArray + markCov + baseCov.
	headerLen := 12
	markArrayOff := headerLen
	baseArrayOff := markArrayOff + len(markArray)
	markCovOff := baseArrayOff + len(baseArray)
	baseCovOff := markCovOff + len(markCov)

	var b []byte
	encU16(&b, 1) // format
	encU16(&b, uint16(markCovOff))
	encU16(&b, uint16(baseCovOff))
	encU16(&b, uint16(markClassCount))
	encU16(&b, uint16(markArrayOff))
	encU16(&b, uint16(baseArrayOff))
	b = append(b, markArray...)
	b = append(b, baseArray...)
	b = append(b, markCov...)
	b = append(b, baseCov...)
	return b
}

func TestGPOSMarkToBase(t *testing.T) {
	// Two marks (gid 100, 101), two bases (gid 50, 51). Two classes.
	// mark 100 -> class 0 with anchor (0, -50)
	// mark 101 -> class 1 with anchor (0, +80)
	// base 50 class0 anchor (10, 800), class1 anchor (10, 100)
	// base 51 class0 anchor (20, 900), class1 anchor (20, 200)
	sub := buildMarkToBase(
		[]uint16{100, 101},
		[]uint16{50, 51},
		2,
		[]struct {
			class  uint16
			anchor Anchor
		}{
			{class: 0, anchor: Anchor{X: 0, Y: -50}},
			{class: 1, anchor: Anchor{X: 0, Y: 80}},
		},
		[][]*Anchor{
			{{X: 10, Y: 800}, {X: 10, Y: 100}},
			{{X: 20, Y: 900}, {X: 20, Y: 200}},
		},
	)
	data := buildGPOSWithSubtable(4, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	// mark 100 on base 50 -> class 0 -> base anchor (10, 800), mark anchor (0, -50).
	att, ok := tbl.MarkToBase(0, 100, 50)
	if !ok {
		t.Fatal("MarkToBase(100, 50) ok=false")
	}
	if att.BaseAnchor.X != 10 || att.BaseAnchor.Y != 800 {
		t.Errorf("base anchor: got %+v, want (10, 800)", att.BaseAnchor)
	}
	if att.MarkAnchor.X != 0 || att.MarkAnchor.Y != -50 {
		t.Errorf("mark anchor: got %+v, want (0, -50)", att.MarkAnchor)
	}

	// mark 101 on base 51 -> class 1 -> (20, 200), (0, 80).
	att, ok = tbl.MarkToBase(0, 101, 51)
	if !ok {
		t.Fatal("MarkToBase(101, 51) ok=false")
	}
	if att.BaseAnchor.X != 20 || att.BaseAnchor.Y != 200 {
		t.Errorf("base anchor: got %+v, want (20, 200)", att.BaseAnchor)
	}

	// Uncovered mark.
	if _, ok := tbl.MarkToBase(0, 999, 50); ok {
		t.Error("uncovered mark: ok=true, want false")
	}
	// Uncovered base.
	if _, ok := tbl.MarkToBase(0, 100, 999); ok {
		t.Error("uncovered base: ok=true, want false")
	}
}

func TestGPOSMarkToBaseMissingAnchorForClass(t *testing.T) {
	// Mark class 1, but base only has anchors for class 0. Expect ok=false.
	sub := buildMarkToBase(
		[]uint16{100},
		[]uint16{50},
		2,
		[]struct {
			class  uint16
			anchor Anchor
		}{
			{class: 1, anchor: Anchor{X: 0, Y: -50}},
		},
		[][]*Anchor{
			{{X: 10, Y: 800}, nil}, // class 0 present, class 1 absent
		},
	)
	data := buildGPOSWithSubtable(4, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tbl.MarkToBase(0, 100, 50); ok {
		t.Error("expected ok=false when base lacks an anchor for the mark's class")
	}
}

func TestGPOSMarkToMark(t *testing.T) {
	// Same subtable shape as mark-to-base, just a different lookup type.
	sub := buildMarkToBase(
		[]uint16{300},
		[]uint16{200},
		1,
		[]struct {
			class  uint16
			anchor Anchor
		}{
			{class: 0, anchor: Anchor{X: 5, Y: -30}},
		},
		[][]*Anchor{
			{{X: 15, Y: 120}},
		},
	)
	data := buildGPOSWithSubtable(6, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	att, ok := tbl.MarkToMark(0, 300, 200)
	if !ok {
		t.Fatal("MarkToMark: ok=false")
	}
	if att.BaseAnchor != (Anchor{X: 15, Y: 120}) {
		t.Errorf("base anchor: got %+v, want (15, 120)", att.BaseAnchor)
	}
	if att.MarkAnchor != (Anchor{X: 5, Y: -30}) {
		t.Errorf("mark anchor: got %+v, want (5, -30)", att.MarkAnchor)
	}
	// MarkToBase on a Type-6 lookup should miss.
	if _, ok := tbl.MarkToBase(0, 300, 200); ok {
		t.Error("MarkToBase on Type-6 lookup: ok=true, want false")
	}
}
