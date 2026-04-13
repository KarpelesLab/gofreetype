// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// MarkAttachment is the result of a mark-to-base or mark-to-mark lookup:
// the anchor on the "base" and the anchor on the mark. A shaper aligns
// mark.Base with the base glyph's anchor, then applies the mark-anchor
// offset to place the mark.
type MarkAttachment struct {
	BaseAnchor Anchor
	MarkAnchor Anchor
}

// MarkToBase looks up the anchors connecting mark glyph `mark` to base
// glyph `base` via the given Type-4 Lookup. ok is false when the lookup
// is not mark-to-base, or either glyph is not covered, or the mark class
// has no anchor on the base.
func (t *Table) MarkToBase(lookupIndex uint16, mark, base uint16) (MarkAttachment, bool) {
	return t.markAttach(lookupIndex, 4, mark, base)
}

// MarkToMark looks up mark-to-mark (Type 6) anchors. A shaper uses this
// for stacked diacritics: mark combines with a preceding mark, not a base.
func (t *Table) MarkToMark(lookupIndex uint16, mark, preceding uint16) (MarkAttachment, bool) {
	return t.markAttach(lookupIndex, 6, mark, preceding)
}

func (t *Table) markAttach(lookupIndex uint16, wantType uint16, mark, base uint16) (MarkAttachment, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return MarkAttachment{}, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != wantType {
		return MarkAttachment{}, false
	}
	for _, sub := range subtables {
		att, found, err := lookupMarkAttach(sub, mark, base)
		if err != nil || !found {
			continue
		}
		return att, true
	}
	return MarkAttachment{}, false
}

// lookupMarkAttach implements the shared body of Types 4/5/6. (Type 5 has
// an extra "component" dimension — we dispatch to lookupMarkAttach only
// for the base/base case; mark-to-ligature is handled separately.)
func lookupMarkAttach(sub []byte, mark, base uint16) (MarkAttachment, bool, error) {
	if len(sub) < 12 {
		return MarkAttachment{}, false, FormatError("mark-attach subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return MarkAttachment{}, false, UnsupportedError("mark-attach format " + intCount(int(format)))
	}
	markCovOff := int(u16(sub, 2))
	baseCovOff := int(u16(sub, 4))
	markClassCount := int(u16(sub, 6))
	markArrayOff := int(u16(sub, 8))
	baseArrayOff := int(u16(sub, 10))

	markCov, err := layout.ParseCoverage(sub, markCovOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}
	baseCov, err := layout.ParseCoverage(sub, baseCovOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}
	markIdx := markCov.Index(mark)
	if markIdx < 0 {
		return MarkAttachment{}, false, nil
	}
	baseIdx := baseCov.Index(base)
	if baseIdx < 0 {
		return MarkAttachment{}, false, nil
	}

	// MarkArray: markCount (u16) + markCount * (markClass u16, anchorOff u16)
	if markArrayOff+2 > len(sub) {
		return MarkAttachment{}, false, FormatError("MarkArray header truncated")
	}
	markCount := int(u16(sub, markArrayOff))
	if markArrayOff+2+4*markCount > len(sub) {
		return MarkAttachment{}, false, FormatError("MarkArray records truncated")
	}
	if markIdx >= markCount {
		return MarkAttachment{}, false, FormatError("mark index out of MarkArray")
	}
	markRec := markArrayOff + 2 + 4*markIdx
	markClass := int(u16(sub, markRec))
	markAnchorOff := int(u16(sub, markRec+2))
	if markClass >= markClassCount {
		return MarkAttachment{}, false, FormatError("markClass out of range")
	}
	markAnchor, _, err := parseAnchor(sub, markArrayOff+markAnchorOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}

	// BaseArray: baseCount (u16) + baseCount * (markClassCount * anchorOff)
	if baseArrayOff+2 > len(sub) {
		return MarkAttachment{}, false, FormatError("BaseArray header truncated")
	}
	baseCount := int(u16(sub, baseArrayOff))
	if baseIdx >= baseCount {
		return MarkAttachment{}, false, FormatError("base index out of BaseArray")
	}
	recStride := 2 * markClassCount
	baseRec := baseArrayOff + 2 + baseIdx*recStride
	if baseRec+2*markClassCount > len(sub) {
		return MarkAttachment{}, false, FormatError("BaseArray row out of bounds")
	}
	baseAnchorOff := int(u16(sub, baseRec+2*markClass))
	if baseAnchorOff == 0 {
		// No anchor for this mark class on this base.
		return MarkAttachment{}, false, nil
	}
	baseAnchor, _, err := parseAnchor(sub, baseArrayOff+baseAnchorOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}

	return MarkAttachment{BaseAnchor: baseAnchor, MarkAnchor: markAnchor}, true, nil
}
