// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

// Context and chaining-context subtables in GSUB (Types 5 + 6) and GPOS
// (Types 7 + 8) share an identical on-disk format. Factor the matching
// engine here so both consumer packages dispatch to a single
// implementation that just operates on the raw subtable bytes.

// SequenceLookupRecord is one nested lookup invocation produced by a
// matching context/chaining-context rule.
type SequenceLookupRecord struct {
	SequenceIndex   uint16
	LookupListIndex uint16
}

// MatchContextSubtable tries to match a Context (GSUB Type 5 / GPOS Type 7)
// subtable at position `start` in `glyphs`. On match, it returns the
// nested actions and the number of input glyphs consumed.
func MatchContextSubtable(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 2 {
		return nil, 0, false, FormatError("context subtable too short")
	}
	switch u16(sub, 0) {
	case 1:
		return matchContextFormat1(sub, glyphs, start)
	case 2:
		return matchContextFormat2(sub, glyphs, start)
	case 3:
		return matchContextFormat3(sub, glyphs, start)
	}
	return nil, 0, false, UnsupportedError("context format")
}

// MatchChainingContextSubtable tries to match a Chaining Context (GSUB Type 6
// / GPOS Type 8) subtable at position `start` in `glyphs`.
func MatchChainingContextSubtable(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 2 {
		return nil, 0, false, FormatError("chaining-context subtable too short")
	}
	switch u16(sub, 0) {
	case 1:
		return matchChainingContextFormat1(sub, glyphs, start)
	case 2:
		return matchChainingContextFormat2(sub, glyphs, start)
	case 3:
		return matchChainingContextFormat3(sub, glyphs, start)
	}
	return nil, 0, false, UnsupportedError("chaining-context format")
}

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

// -------------------- Context (Type 5 / 7) --------------------

func matchContextFormat1(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 6 || start < 0 || start >= len(glyphs) {
		return nil, 0, false, nil
	}
	covOff := int(u16(sub, 2))
	n := int(u16(sub, 4))
	if 6+2*n > len(sub) {
		return nil, 0, false, FormatError("ruleSetOffsets truncated")
	}
	cov, err := ParseCoverage(sub, covOff)
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

func matchContextFormat2(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 8 || start < 0 || start >= len(glyphs) {
		return nil, 0, false, nil
	}
	covOff := int(u16(sub, 2))
	cdOff := int(u16(sub, 4))
	nSets := int(u16(sub, 6))
	if 8+2*nSets > len(sub) {
		return nil, 0, false, FormatError("classSetOffsets truncated")
	}
	cov, err := ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	if cov.Index(glyphs[start]) < 0 {
		return nil, 0, false, nil
	}
	cd, err := ParseClassDef(sub, cdOff)
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

func evalContextRuleClass(sub []byte, ruleOff int, glyphs []uint16, start int, cd *ClassDef) ([]SequenceLookupRecord, int, bool, error) {
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
		cov, err := ParseCoverage(sub, int(u16(sub, 4+2*i)))
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

// -------------------- Chaining Context (Type 6 / 8) --------------------

func matchChainingContextFormat1(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 6 || start < 0 || start >= len(glyphs) {
		return nil, 0, false, nil
	}
	covOff := int(u16(sub, 2))
	n := int(u16(sub, 4))
	if 6+2*n > len(sub) {
		return nil, 0, false, FormatError("chainRuleSetOffsets truncated")
	}
	cov, err := ParseCoverage(sub, covOff)
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
		return nil, 0, false, FormatError("chain rule backtrack truncated")
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
	if p+2*(nInput-1) > len(sub) {
		return nil, 0, false, FormatError("chain rule input truncated")
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
	if start < nBack || start+nInput > len(glyphs) || start+nInput+nAhead > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nBack; i++ {
		if glyphs[start-1-i] != u16(sub, backStart+2*i) {
			return nil, 0, false, nil
		}
	}
	for i := 0; i < nInput-1; i++ {
		if glyphs[start+1+i] != u16(sub, inputStart+2*i) {
			return nil, 0, false, nil
		}
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

func matchChainingContextFormat2(sub []byte, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool, error) {
	if len(sub) < 12 || start < 0 || start >= len(glyphs) {
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
	cov, err := ParseCoverage(sub, covOff)
	if err != nil {
		return nil, 0, false, err
	}
	if cov.Index(glyphs[start]) < 0 {
		return nil, 0, false, nil
	}
	inputCD, err := ParseClassDef(sub, inputCDOff)
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
	backCD, err := ParseClassDef(sub, backCDOff)
	if err != nil {
		return nil, 0, false, err
	}
	aheadCD, err := ParseClassDef(sub, aheadCDOff)
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
	backCD, inputCD, aheadCD *ClassDef,
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
		return nil, 0, false, FormatError("chaining-context format 3 truncated (action count)")
	}
	nActions := int(u16(sub, p))
	p += 2
	if start < 0 || start+nInput > len(glyphs) {
		return nil, 0, false, nil
	}
	if start < nBack || start+nInput+nAhead > len(glyphs) {
		return nil, 0, false, nil
	}
	for i := 0; i < nBack; i++ {
		cov, err := ParseCoverage(sub, backOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start-1-i]) < 0 {
			return nil, 0, false, nil
		}
	}
	for i := 0; i < nInput; i++ {
		cov, err := ParseCoverage(sub, inputOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start+i]) < 0 {
			return nil, 0, false, nil
		}
	}
	for i := 0; i < nAhead; i++ {
		cov, err := ParseCoverage(sub, aheadOffs[i])
		if err != nil {
			return nil, 0, false, err
		}
		if cov.Index(glyphs[start+nInput+i]) < 0 {
			return nil, 0, false, nil
		}
	}
	acts, err := readActionList(sub, p, nActions)
	if err != nil {
		return nil, 0, false, err
	}
	return acts, nInput, true, nil
}
