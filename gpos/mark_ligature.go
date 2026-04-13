// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// MarkToLigature (GPOS Type 5) attaches a mark to a specific component
// of a ligature glyph. The shaper must track which component of a
// ligature the mark is "for" - typically derived from the cluster
// information produced by GSUB ligature substitution.
//
// On-disk layout mirrors Type 4 except the BaseArray is replaced by
// LigatureArray, which is indirect: each ligature has a LigatureAttach
// table holding per-component anchor rows.
func (t *Table) MarkToLigature(lookupIndex uint16, mark, ligature uint16, component int) (MarkAttachment, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return MarkAttachment{}, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 5 {
		return MarkAttachment{}, false
	}
	for _, sub := range subtables {
		att, found, err := lookupMarkToLigature(sub, mark, ligature, component)
		if err != nil || !found {
			continue
		}
		return att, true
	}
	return MarkAttachment{}, false
}

func lookupMarkToLigature(sub []byte, mark, ligature uint16, component int) (MarkAttachment, bool, error) {
	if len(sub) < 12 {
		return MarkAttachment{}, false, FormatError("mark-to-lig subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return MarkAttachment{}, false, UnsupportedError("mark-to-lig format " + intCount(int(format)))
	}
	markCovOff := int(u16(sub, 2))
	ligCovOff := int(u16(sub, 4))
	markClassCount := int(u16(sub, 6))
	markArrayOff := int(u16(sub, 8))
	ligArrayOff := int(u16(sub, 10))

	markCov, err := layout.ParseCoverage(sub, markCovOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}
	ligCov, err := layout.ParseCoverage(sub, ligCovOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}
	markIdx := markCov.Index(mark)
	if markIdx < 0 {
		return MarkAttachment{}, false, nil
	}
	ligIdx := ligCov.Index(ligature)
	if ligIdx < 0 {
		return MarkAttachment{}, false, nil
	}

	// MarkArray — same layout as Type 4.
	if markArrayOff+2 > len(sub) {
		return MarkAttachment{}, false, FormatError("MarkArray truncated")
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

	// LigatureArray.
	if ligArrayOff+2 > len(sub) {
		return MarkAttachment{}, false, FormatError("LigatureArray truncated")
	}
	ligCount := int(u16(sub, ligArrayOff))
	if ligIdx >= ligCount {
		return MarkAttachment{}, false, FormatError("ligature index out of LigatureArray")
	}
	if ligArrayOff+2+2*ligCount > len(sub) {
		return MarkAttachment{}, false, FormatError("LigatureArray offsets truncated")
	}
	ligAttachOff := int(u16(sub, ligArrayOff+2+2*ligIdx))
	ligAttachAbs := ligArrayOff + ligAttachOff
	if ligAttachAbs+2 > len(sub) {
		return MarkAttachment{}, false, FormatError("LigatureAttach truncated")
	}
	componentCount := int(u16(sub, ligAttachAbs))
	if component < 0 || component >= componentCount {
		return MarkAttachment{}, false, nil
	}
	compRecStride := 2 * markClassCount
	compRec := ligAttachAbs + 2 + component*compRecStride
	if compRec+compRecStride > len(sub) {
		return MarkAttachment{}, false, FormatError("component record out of bounds")
	}
	ligAnchorOff := int(u16(sub, compRec+2*markClass))
	if ligAnchorOff == 0 {
		return MarkAttachment{}, false, nil
	}
	baseAnchor, _, err := parseAnchor(sub, ligAttachAbs+ligAnchorOff)
	if err != nil {
		return MarkAttachment{}, false, err
	}
	return MarkAttachment{BaseAnchor: baseAnchor, MarkAnchor: markAnchor}, true, nil
}
