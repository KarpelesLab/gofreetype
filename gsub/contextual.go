// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// SequenceLookupRecord describes one nested lookup invocation produced by
// a matching contextual rule. The shaper should apply lookup
// t.Lookups[LookupListIndex] at position (start + SequenceIndex) within
// the input glyph run.
type SequenceLookupRecord struct {
	SequenceIndex   uint16
	LookupListIndex uint16
}

// MatchChainingContext tries to match a Chaining Context (Type 6) rule
// starting at position `start` in `glyphs`. If a rule in the named Lookup
// matches, it returns the nested lookup actions and the number of input
// glyphs the match consumed (backtrack and lookahead do NOT consume
// glyphs — they only constrain context).
//
// This method handles Format 3 (per-position Coverage tables). Formats 1
// (glyph-sequence) and 2 (class-sequence) will be added in a follow-up
// commit.
func (t *Table) MatchChainingContext(lookupIndex uint16, glyphs []uint16, start int) (actions []SequenceLookupRecord, consumed int, ok bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 6 {
		return nil, 0, false
	}
	for _, sub := range subtables {
		acts, n, found, err := matchChainingContext(sub, glyphs, start)
		if err != nil || !found {
			continue
		}
		return acts, n, true
	}
	return nil, 0, false
}

func matchChainingContext(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 2 {
		return nil, 0, false, FormatError("chaining-context subtable too short")
	}
	format := u16(sub, 0)
	switch format {
	case 3:
		return matchChainingContextFormat3(sub, glyphs, start)
	case 1, 2:
		return nil, 0, false, UnsupportedError("chaining-context format " + intToStr(int(format)))
	}
	return nil, 0, false, UnsupportedError("chaining-context format " + intToStr(int(format)))
}

// matchChainingContextFormat3 handles the coverage-based Chaining Context
// subtable:
//
//	uint16 format (3)
//	uint16 backtrackGlyphCount
//	Offset16 backtrackCoverageOffsets[backtrackGlyphCount]
//	uint16 inputGlyphCount
//	Offset16 inputCoverageOffsets[inputGlyphCount]
//	uint16 lookaheadGlyphCount
//	Offset16 lookaheadCoverageOffsets[lookaheadGlyphCount]
//	uint16 seqLookupCount
//	SequenceLookupRecord seqLookupRecords[seqLookupCount]
//
// Backtrack offsets are stored in forward order on disk, but the first
// entry is matched against glyphs[start-1], the second against
// glyphs[start-2], etc.
func matchChainingContextFormat3(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	p := 2
	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (backtrack count)")
	}
	nBack := int(u16(sub, p))
	p += 2
	if p+2*nBack > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (backtrack offsets)")
	}
	backOffs := make([]int, nBack)
	for i := 0; i < nBack; i++ {
		backOffs[i] = int(u16(sub, p+2*i))
	}
	p += 2 * nBack

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (input count)")
	}
	nInput := int(u16(sub, p))
	p += 2
	if p+2*nInput > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (input offsets)")
	}
	inputOffs := make([]int, nInput)
	for i := 0; i < nInput; i++ {
		inputOffs[i] = int(u16(sub, p+2*i))
	}
	p += 2 * nInput

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (lookahead count)")
	}
	nAhead := int(u16(sub, p))
	p += 2
	if p+2*nAhead > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (lookahead offsets)")
	}
	aheadOffs := make([]int, nAhead)
	for i := 0; i < nAhead; i++ {
		aheadOffs[i] = int(u16(sub, p+2*i))
	}
	p += 2 * nAhead

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 truncated (seqLookupCount)")
	}
	nActions := int(u16(sub, p))
	p += 2
	if p+4*nActions > len(sub) {
		return nil, 0, false, FormatError("chaining-context format 3 seqLookupRecords truncated")
	}

	// Bounds check.
	if start < 0 || start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	if start < nBack {
		return nil, 0, false, nil
	}
	if start+nInput+nAhead > len(glyphs) {
		return nil, 0, false, nil
	}

	// Match backtrack (reversed).
	for i := 0; i < nBack; i++ {
		cov, err := layout.ParseCoverage(sub, backOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start-1-i]) < 0 {
			return nil, 0, false, nil
		}
	}
	// Match input (forward).
	for i := 0; i < nInput; i++ {
		cov, err := layout.ParseCoverage(sub, inputOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start+i]) < 0 {
			return nil, 0, false, nil
		}
	}
	// Match lookahead (forward from end of input).
	for i := 0; i < nAhead; i++ {
		cov, err := layout.ParseCoverage(sub, aheadOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start+nInput+i]) < 0 {
			return nil, 0, false, nil
		}
	}

	actions := make([]SequenceLookupRecord, nActions)
	for i := 0; i < nActions; i++ {
		actions[i] = SequenceLookupRecord{
			SequenceIndex:   u16(sub, p+4*i),
			LookupListIndex: u16(sub, p+4*i+2),
		}
	}
	return actions, nInput, true, nil
}
