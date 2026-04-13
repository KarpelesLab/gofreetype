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
	case 1:
		return matchChainingContextFormat1(sub, glyphs, start)
	case 2:
		return matchChainingContextFormat2(sub, glyphs, start)
	case 3:
		return matchChainingContextFormat3(sub, glyphs, start)
	}
	return nil, 0, false, UnsupportedError("chaining-context format " + intToStr(int(format)))
}

// MatchContext tries to match a Type-5 Context rule starting at position
// `start` in `glyphs`. Like MatchChainingContext but with no backtrack /
// lookahead constraints.
func (t *Table) MatchContext(lookupIndex uint16, glyphs []uint16, start int) (actions []SequenceLookupRecord, consumed int, ok bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 5 {
		return nil, 0, false
	}
	for _, sub := range subtables {
		acts, n, found, err := matchContext(sub, glyphs, start)
		if err != nil || !found {
			continue
		}
		return acts, n, true
	}
	return nil, 0, false
}

func matchContext(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 2 {
		return nil, 0, false, FormatError("context subtable too short")
	}
	format := u16(sub, 0)
	switch format {
	case 1:
		return matchContextFormat1(sub, glyphs, start)
	case 2:
		return matchContextFormat2(sub, glyphs, start)
	case 3:
		return matchContextFormat3(sub, glyphs, start)
	}
	return nil, 0, false, UnsupportedError("context format " + intToStr(int(format)))
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

// readActionList reads nActions 4-byte (SequenceIndex, LookupListIndex)
// records starting at sub[off:].
func readActionList(sub []byte, off, nActions int) ([]SequenceLookupRecord, error) {
	if off+4*nActions > len(sub) {
		return nil, FormatError("action list out of bounds")
	}
	out := make([]SequenceLookupRecord, nActions)
	for i := 0; i < nActions; i++ {
		out[i] = SequenceLookupRecord{
			SequenceIndex:   u16(sub, off+4*i),
			LookupListIndex: u16(sub, off+4*i+2),
		}
	}
	return out, nil
}

// --- Chaining Context Format 1 (glyph-sequence) ---
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 chainRuleSetCount
//	Offset16 chainRuleSetOffsets[chainRuleSetCount]
//
// ChainRuleSet:
//	uint16 chainRuleCount
//	Offset16 chainRuleOffsets[chainRuleCount]
//
// ChainRule:
//	uint16 backtrackGlyphCount
//	uint16 backtrackSequence[...]
//	uint16 inputGlyphCount
//	uint16 inputSequence[inputGlyphCount - 1]  (first input = coverage glyph)
//	uint16 lookaheadGlyphCount
//	uint16 lookaheadSequence[...]
//	uint16 seqLookupCount
//	SequenceLookupRecord seqLookupRecords[...]
func matchChainingContextFormat1(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 6 {
		return nil, 0, false, FormatError("chaining-context format 1 header truncated")
	}
	if start < 0 || start >= len(glyphs) {
		return nil, 0, false, nil
	}
	covOff := int(u16(sub, 2))
	n := int(u16(sub, 4))
	if 6+2*n > len(sub) {
		return nil, 0, false, FormatError("chainRuleSetOffsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	idx := cov.Index(glyphs[start])
	if idx < 0 || idx >= n {
		return nil, 0, false, nil
	}
	setOff := int(u16(sub, 6+2*idx))
	if setOff+2 > len(sub) {
		return nil, 0, false, FormatError("chainRuleSet header truncated")
	}
	nRules := int(u16(sub, setOff))
	if setOff+2+2*nRules > len(sub) {
		return nil, 0, false, FormatError("chainRuleOffsets truncated")
	}
	for i := 0; i < nRules; i++ {
		ruleOff := setOff + int(u16(sub, setOff+2+2*i))
		acts, consumed, found, err := evalChainRuleGlyph(sub, ruleOff, glyphs, start)
		if err != nil {
			return nil, 0, false, err
		}
		if found {
			return acts, consumed, true, nil
		}
	}
	return nil, 0, false, nil
}

func evalChainRuleGlyph(sub []byte, ruleOff int, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	p := ruleOff
	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain rule truncated (backtrack count)")
	}
	nBack := int(u16(sub, p))
	p += 2
	if p+2*nBack > len(sub) {
		return nil, 0, false, FormatError("chain rule backtrack sequence truncated")
	}
	backStart := p
	p += 2 * nBack

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain rule truncated (input count)")
	}
	nInput := int(u16(sub, p))
	p += 2
	if nInput < 1 {
		return nil, 0, false, FormatError("chain rule input count must be >= 1")
	}
	// inputSequence has nInput-1 entries (first matched by coverage).
	if p+2*(nInput-1) > len(sub) {
		return nil, 0, false, FormatError("chain rule input sequence truncated")
	}
	inputStart := p
	p += 2 * (nInput - 1)

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain rule truncated (lookahead count)")
	}
	nAhead := int(u16(sub, p))
	p += 2
	if p+2*nAhead > len(sub) {
		return nil, 0, false, FormatError("chain rule lookahead truncated")
	}
	aheadStart := p
	p += 2 * nAhead

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain rule truncated (action count)")
	}
	nActions := int(u16(sub, p))
	p += 2

	// Match backtrack (reversed).
	if start < nBack {
		return nil, 0, false, nil
	}
	for i := 0; i < nBack; i++ {
		if glyphs[start-1-i] != u16(sub, backStart+2*i) {
			return nil, 0, false, nil
		}
	}
	// Match input (starting at start+1 since glyphs[start] is the coverage match).
	if start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nInput-1; i++ {
		if glyphs[start+1+i] != u16(sub, inputStart+2*i) {
			return nil, 0, false, nil
		}
	}
	// Match lookahead.
	if start+nInput+nAhead > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nAhead; i++ {
		if glyphs[start+nInput+i] != u16(sub, aheadStart+2*i) {
			return nil, 0, false, nil
		}
	}
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}

// --- Chaining Context Format 2 (class-sequence) ---
//
//	uint16 format (2)
//	Offset16 coverageOffset
//	Offset16 backtrackClassDefOffset
//	Offset16 inputClassDefOffset
//	Offset16 lookaheadClassDefOffset
//	uint16 chainClassSetCount
//	Offset16 chainClassSetOffsets[chainClassSetCount]  (may be 0 = no rules)
//
// ChainClassSet and ChainClassRule mirror the glyph variant but carry
// class values instead of glyph ids.
func matchChainingContextFormat2(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 12 {
		return nil, 0, false, FormatError("chaining-context format 2 header truncated")
	}
	if start < 0 || start >= len(glyphs) {
		return nil, 0, false, nil
	}
	covOff := int(u16(sub, 2))
	backCDOff := int(u16(sub, 4))
	inputCDOff := int(u16(sub, 6))
	aheadCDOff := int(u16(sub, 8))
	nSets := int(u16(sub, 10))
	if 12+2*nSets > len(sub) {
		return nil, 0, false, FormatError("chainClassSetOffsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	if cov.Index(glyphs[start]) < 0 {
		return nil, 0, false, nil
	}
	inputCD, err := layout.ParseClassDef(sub, inputCDOff)
	if err != nil {
		return nil, 0, false, err
	}
	startClass := int(inputCD.Class(glyphs[start]))
	if startClass >= nSets {
		return nil, 0, false, nil
	}
	setOff := int(u16(sub, 12+2*startClass))
	if setOff == 0 {
		return nil, 0, false, nil
	}
	if setOff+2 > len(sub) {
		return nil, 0, false, FormatError("chainClassSet header truncated")
	}
	nRules := int(u16(sub, setOff))
	if setOff+2+2*nRules > len(sub) {
		return nil, 0, false, FormatError("chainClassRuleOffsets truncated")
	}
	backCD, err := layout.ParseClassDef(sub, backCDOff)
	if err != nil {
		return nil, 0, false, err
	}
	aheadCD, err := layout.ParseClassDef(sub, aheadCDOff)
	if err != nil {
		return nil, 0, false, err
	}
	for i := 0; i < nRules; i++ {
		ruleOff := setOff + int(u16(sub, setOff+2+2*i))
		acts, consumed, found, err := evalChainRuleClass(sub, ruleOff, glyphs, start, backCD, inputCD, aheadCD)
		if err != nil {
			return nil, 0, false, err
		}
		if found {
			return acts, consumed, true, nil
		}
	}
	return nil, 0, false, nil
}

func evalChainRuleClass(sub []byte, ruleOff int, glyphs []uint16, start int,
	backCD, inputCD, aheadCD *layout.ClassDef,
) ([]SequenceLookupRecord, int, bool, error) {
	p := ruleOff
	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain class rule truncated (backtrack count)")
	}
	nBack := int(u16(sub, p))
	p += 2
	if p+2*nBack > len(sub) {
		return nil, 0, false, FormatError("chain class rule backtrack truncated")
	}
	backStart := p
	p += 2 * nBack

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain class rule truncated (input count)")
	}
	nInput := int(u16(sub, p))
	p += 2
	if nInput < 1 {
		return nil, 0, false, FormatError("chain class rule input count must be >= 1")
	}
	if p+2*(nInput-1) > len(sub) {
		return nil, 0, false, FormatError("chain class rule input truncated")
	}
	inputStart := p
	p += 2 * (nInput - 1)

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain class rule truncated (lookahead count)")
	}
	nAhead := int(u16(sub, p))
	p += 2
	if p+2*nAhead > len(sub) {
		return nil, 0, false, FormatError("chain class rule lookahead truncated")
	}
	aheadStart := p
	p += 2 * nAhead

	if p+2 > len(sub) {
		return nil, 0, false, FormatError("chain class rule truncated (action count)")
	}
	nActions := int(u16(sub, p))
	p += 2

	if start < nBack || start+nInput > len(glyphs) || start+nInput+nAhead > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nBack; i++ {
		if uint16(backCD.Class(glyphs[start-1-i])) != u16(sub, backStart+2*i) {
			return nil, 0, false, nil
		}
	}
	for i := 0; i < nInput-1; i++ {
		if uint16(inputCD.Class(glyphs[start+1+i])) != u16(sub, inputStart+2*i) {
			return nil, 0, false, nil
		}
	}
	for i := 0; i < nAhead; i++ {
		if uint16(aheadCD.Class(glyphs[start+nInput+i])) != u16(sub, aheadStart+2*i) {
			return nil, 0, false, nil
		}
	}
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}

// --- Context (Type 5) Format 1 (glyph sequence) ---
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 ruleSetCount
//	Offset16 ruleSetOffsets[ruleSetCount]
//
// RuleSet:
//	uint16 ruleCount
//	Offset16 ruleOffsets[ruleCount]
//
// Rule:
//	uint16 glyphCount
//	uint16 inputSequence[glyphCount - 1]
//	uint16 seqLookupCount
//	SequenceLookupRecord[seqLookupCount]
func matchContextFormat1(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 6 || start < 0 || start >= len(glyphs) {
		return nil, 0, false, FormatError("context format 1 header truncated")
	}
	covOff := int(u16(sub, 2))
	n := int(u16(sub, 4))
	if 6+2*n > len(sub) {
		return nil, 0, false, FormatError("ruleSetOffsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	idx := cov.Index(glyphs[start])
	if idx < 0 || idx >= n {
		return nil, 0, false, nil
	}
	setOff := int(u16(sub, 6+2*idx))
	if setOff+2 > len(sub) {
		return nil, 0, false, FormatError("ruleSet header truncated")
	}
	nRules := int(u16(sub, setOff))
	if setOff+2+2*nRules > len(sub) {
		return nil, 0, false, FormatError("ruleOffsets truncated")
	}
	for i := 0; i < nRules; i++ {
		ruleOff := setOff + int(u16(sub, setOff+2+2*i))
		acts, consumed, found, err := evalContextRuleGlyph(sub, ruleOff, glyphs, start)
		if err != nil {
			return nil, 0, false, err
		}
		if found {
			return acts, consumed, true, nil
		}
	}
	return nil, 0, false, nil
}

func evalContextRuleGlyph(sub []byte, ruleOff int, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	p := ruleOff
	if p+4 > len(sub) {
		return nil, 0, false, FormatError("context rule header truncated")
	}
	nInput := int(u16(sub, p))
	nActions := int(u16(sub, p+2))
	if nInput < 1 {
		return nil, 0, false, FormatError("context rule input count must be >= 1")
	}
	p += 4
	if p+2*(nInput-1) > len(sub) {
		return nil, 0, false, FormatError("context rule input truncated")
	}
	if start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nInput-1; i++ {
		if glyphs[start+1+i] != u16(sub, p+2*i) {
			return nil, 0, false, nil
		}
	}
	p += 2 * (nInput - 1)
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}

// --- Context (Type 5) Format 2 (class sequence) ---
func matchContextFormat2(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 8 || start < 0 || start >= len(glyphs) {
		return nil, 0, false, FormatError("context format 2 header truncated")
	}
	covOff := int(u16(sub, 2))
	cdOff := int(u16(sub, 4))
	nSets := int(u16(sub, 6))
	if 8+2*nSets > len(sub) {
		return nil, 0, false, FormatError("classSetOffsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	if cov.Index(glyphs[start]) < 0 {
		return nil, 0, false, nil
	}
	cd, err := layout.ParseClassDef(sub, cdOff)
	if err != nil {
		return nil, 0, false, err
	}
	startClass := int(cd.Class(glyphs[start]))
	if startClass >= nSets {
		return nil, 0, false, nil
	}
	setOff := int(u16(sub, 8+2*startClass))
	if setOff == 0 {
		return nil, 0, false, nil
	}
	if setOff+2 > len(sub) {
		return nil, 0, false, FormatError("classSet header truncated")
	}
	nRules := int(u16(sub, setOff))
	if setOff+2+2*nRules > len(sub) {
		return nil, 0, false, FormatError("classRuleOffsets truncated")
	}
	for i := 0; i < nRules; i++ {
		ruleOff := setOff + int(u16(sub, setOff+2+2*i))
		acts, consumed, found, err := evalContextRuleClass(sub, ruleOff, glyphs, start, cd)
		if err != nil {
			return nil, 0, false, err
		}
		if found {
			return acts, consumed, true, nil
		}
	}
	return nil, 0, false, nil
}

func evalContextRuleClass(sub []byte, ruleOff int, glyphs []uint16, start int, cd *layout.ClassDef) ([]SequenceLookupRecord, int, bool, error) {
	p := ruleOff
	if p+4 > len(sub) {
		return nil, 0, false, FormatError("context class rule header truncated")
	}
	nInput := int(u16(sub, p))
	nActions := int(u16(sub, p+2))
	if nInput < 1 {
		return nil, 0, false, FormatError("context class rule input count must be >= 1")
	}
	p += 4
	if p+2*(nInput-1) > len(sub) {
		return nil, 0, false, FormatError("context class rule input truncated")
	}
	if start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nInput-1; i++ {
		if uint16(cd.Class(glyphs[start+1+i])) != u16(sub, p+2*i) {
			return nil, 0, false, nil
		}
	}
	p += 2 * (nInput - 1)
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}

// --- Context Format 3 (per-position Coverage) ---
func matchContextFormat3(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 4 {
		return nil, 0, false, FormatError("context format 3 header truncated")
	}
	nInput := int(u16(sub, 2))
	if 4+2*nInput > len(sub) {
		return nil, 0, false, FormatError("context format 3 coverage offsets truncated")
	}
	if start < 0 || start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nInput; i++ {
		cov, err := layout.ParseCoverage(sub, int(u16(sub, 4+2*i)))
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start+i]) < 0 {
			return nil, 0, false, nil
		}
	}
	p := 4 + 2*nInput
	if p+2 > len(sub) {
		return nil, 0, false, FormatError("context format 3 action count missing")
	}
	nActions := int(u16(sub, p))
	p += 2
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}
