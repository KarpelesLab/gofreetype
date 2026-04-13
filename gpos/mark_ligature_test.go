// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "testing"

// buildMarkToLigature builds a Type-5 subtable. Each input ligature has an
// array of components, and each component has per-class anchors (nil means
// no anchor for that class on that component).
func buildMarkToLigature(
	markGIDs []uint16,
	ligGIDs []uint16,
	markClassCount int,
	markRecords []struct {
		class  uint16
		anchor Anchor
	},
	perLigature [][][]*Anchor, // [ligature][component][class] -> anchor or nil
) []byte {
	// MarkArray (same as Type 4).
	markArray := make([]byte, 2+4*len(markRecords))
	markArray[0] = byte(len(markRecords) >> 8)
	markArray[1] = byte(len(markRecords))
	for i, r := range markRecords {
		markArray[2+4*i] = byte(r.class >> 8)
		markArray[2+4*i+1] = byte(r.class)
	}
	for i, r := range markRecords {
		aOff := uint16(len(markArray))
		markArray[2+4*i+2] = byte(aOff >> 8)
		markArray[2+4*i+3] = byte(aOff)
		markArray = append(markArray, buildAnchorFormat1(r.anchor.X, r.anchor.Y)...)
	}

	// LigatureArray: count + [ligatureAttachOffset u16]*count + per-lig bodies.
	ligArrayHeader := make([]byte, 2+2*len(perLigature))
	ligArrayHeader[0] = byte(len(perLigature) >> 8)
	ligArrayHeader[1] = byte(len(perLigature))
	ligArrayBody := []byte{}
	for i, components := range perLigature {
		// Attach offset relative to ligArray start.
		attachOff := uint16(len(ligArrayHeader) + len(ligArrayBody))
		ligArrayHeader[2+2*i] = byte(attachOff >> 8)
		ligArrayHeader[2+2*i+1] = byte(attachOff)
		// LigatureAttach: componentCount + componentRecords.
		attachStartInLigArray := len(ligArrayHeader) + len(ligArrayBody)
		header := []byte{byte(len(components) >> 8), byte(len(components))}
		// Placeholder for each component's anchor offset grid.
		compRecords := make([]byte, 2*markClassCount*len(components))
		// Append anchors and record their offsets (relative to LigatureAttach start).
		attachCursor := 2 + len(compRecords) // bytes after end of compRecords
		var anchors []byte
		for ci, row := range components {
			for cc := 0; cc < markClassCount; cc++ {
				if cc < len(row) && row[cc] != nil {
					aOff := uint16(attachCursor + len(anchors))
					compRecords[(ci*markClassCount+cc)*2] = byte(aOff >> 8)
					compRecords[(ci*markClassCount+cc)*2+1] = byte(aOff)
					anchors = append(anchors, buildAnchorFormat1(row[cc].X, row[cc].Y)...)
				}
			}
		}
		ligArrayBody = append(ligArrayBody, header...)
		ligArrayBody = append(ligArrayBody, compRecords...)
		ligArrayBody = append(ligArrayBody, anchors...)
		_ = attachStartInLigArray
	}
	ligArray := append(ligArrayHeader, ligArrayBody...)

	markCov := buildCoverageFormat1(markGIDs)
	ligCov := buildCoverageFormat1(ligGIDs)

	headerLen := 12
	markArrayOff := headerLen
	ligArrayOff := markArrayOff + len(markArray)
	markCovOff := ligArrayOff + len(ligArray)
	ligCovOff := markCovOff + len(markCov)

	var b []byte
	encU16(&b, 1)
	encU16(&b, uint16(markCovOff))
	encU16(&b, uint16(ligCovOff))
	encU16(&b, uint16(markClassCount))
	encU16(&b, uint16(markArrayOff))
	encU16(&b, uint16(ligArrayOff))
	b = append(b, markArray...)
	b = append(b, ligArray...)
	b = append(b, markCov...)
	b = append(b, ligCov...)
	return b
}

func TestGPOSMarkToLigature(t *testing.T) {
	// One mark (gid 500, class 0). One ligature (gid 300) with 3 components.
	// Component 0: anchor for class 0 at (10, 100)
	// Component 1: anchor for class 0 at (110, 100)
	// Component 2: no anchor
	sub := buildMarkToLigature(
		[]uint16{500},
		[]uint16{300},
		1,
		[]struct {
			class  uint16
			anchor Anchor
		}{
			{class: 0, anchor: Anchor{X: 0, Y: -20}},
		},
		[][][]*Anchor{
			{
				{{X: 10, Y: 100}},
				{{X: 110, Y: 100}},
				{nil},
			},
		},
	)
	data := buildGPOSWithSubtable(5, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	for comp, wantX := range map[int]int16{0: 10, 1: 110} {
		att, ok := tbl.MarkToLigature(0, 500, 300, comp)
		if !ok {
			t.Fatalf("MarkToLigature(500, 300, comp=%d): ok=false", comp)
		}
		if att.BaseAnchor.X != wantX || att.BaseAnchor.Y != 100 {
			t.Errorf("comp %d base anchor: got %+v, want (%d, 100)", comp, att.BaseAnchor, wantX)
		}
	}
	// Component without an anchor — expected miss.
	if _, ok := tbl.MarkToLigature(0, 500, 300, 2); ok {
		t.Error("component 2 should have no anchor, got ok=true")
	}
	// Out-of-range component.
	if _, ok := tbl.MarkToLigature(0, 500, 300, 5); ok {
		t.Error("component 5 out of range, got ok=true")
	}
}
