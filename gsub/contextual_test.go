// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "testing"

// buildChainingContextFormat3 builds a GSUB Type-6 Format-3 subtable with
// the given backtrack/input/lookahead coverage tables and nested lookup
// records.
func buildChainingContextFormat3(
	backtrack, input, lookahead [][]uint16,
	actions []SequenceLookupRecord,
) []byte {
	// The header has fixed count fields plus per-array uint16 offsets.
	var b []byte
	encU16(&b, 3)
	encU16(&b, uint16(len(backtrack)))
	backOffStart := len(b)
	for range backtrack {
		encU16(&b, 0) // placeholder
	}
	encU16(&b, uint16(len(input)))
	inputOffStart := len(b)
	for range input {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(lookahead)))
	aheadOffStart := len(b)
	for range lookahead {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(actions)))
	for _, a := range actions {
		encU16(&b, a.SequenceIndex)
		encU16(&b, a.LookupListIndex)
	}

	// Append coverage tables and patch offsets.
	patch := func(offStart int, covs [][]uint16) {
		for i, gids := range covs {
			off := uint16(len(b))
			b[offStart+2*i] = byte(off >> 8)
			b[offStart+2*i+1] = byte(off)
			b = append(b, buildCoverageFormat1(gids)...)
		}
	}
	patch(backOffStart, backtrack)
	patch(inputOffStart, input)
	patch(aheadOffStart, lookahead)
	return b
}

func TestGSUBChainingContextFormat3(t *testing.T) {
	// Match rule: backtrack = [{1}], input = [{2}], lookahead = [{3}].
	// Upon match: apply lookup #5 at sequence index 0.
	sub := buildChainingContextFormat3(
		[][]uint16{{1}},
		[][]uint16{{2}},
		[][]uint16{{3}},
		[]SequenceLookupRecord{{SequenceIndex: 0, LookupListIndex: 5}},
	)
	data := buildGSUBWithSubtable(6, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	// glyphs [1, 2, 3] at start=1 should match.
	actions, consumed, ok := tbl.MatchChainingContext(0, []uint16{1, 2, 3}, 1)
	if !ok {
		t.Fatal("expected match at position 1")
	}
	if consumed != 1 {
		t.Errorf("consumed: got %d, want 1", consumed)
	}
	if len(actions) != 1 || actions[0].LookupListIndex != 5 {
		t.Errorf("actions: got %+v, want [{seq=0, lookup=5}]", actions)
	}

	// glyphs [1, 2, 4]: lookahead fails.
	if _, _, ok := tbl.MatchChainingContext(0, []uint16{1, 2, 4}, 1); ok {
		t.Error("lookahead mismatch should not match")
	}
	// glyphs [9, 2, 3]: backtrack fails.
	if _, _, ok := tbl.MatchChainingContext(0, []uint16{9, 2, 3}, 1); ok {
		t.Error("backtrack mismatch should not match")
	}
	// glyphs [1, 9, 3]: input fails.
	if _, _, ok := tbl.MatchChainingContext(0, []uint16{1, 9, 3}, 1); ok {
		t.Error("input mismatch should not match")
	}
	// start=0 with nBack>0 should fail (not enough backtrack context).
	if _, _, ok := tbl.MatchChainingContext(0, []uint16{1, 2, 3}, 0); ok {
		t.Error("start=0 with required backtrack should not match")
	}
}

// buildReverseChain builds a Type-8 Reverse Chaining Single Substitution
// subtable.
func buildReverseChain(
	cov []uint16,
	backtrack [][]uint16,
	lookahead [][]uint16,
	substitutes []uint16,
) []byte {
	var b []byte
	encU16(&b, 1)
	covOffPos := len(b)
	encU16(&b, 0)
	encU16(&b, uint16(len(backtrack)))
	backOffStart := len(b)
	for range backtrack {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(lookahead)))
	aheadOffStart := len(b)
	for range lookahead {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(substitutes)))
	for _, s := range substitutes {
		encU16(&b, s)
	}

	patch := func(offStart int, covs [][]uint16) {
		for i, gids := range covs {
			off := uint16(len(b))
			b[offStart+2*i] = byte(off >> 8)
			b[offStart+2*i+1] = byte(off)
			b = append(b, buildCoverageFormat1(gids)...)
		}
	}
	// Main coverage at covOffPos.
	mainOff := uint16(len(b))
	b[covOffPos] = byte(mainOff >> 8)
	b[covOffPos+1] = byte(mainOff)
	b = append(b, buildCoverageFormat1(cov)...)

	patch(backOffStart, backtrack)
	patch(aheadOffStart, lookahead)
	return b
}

// buildContextFormat3 builds a Type-5 Format-3 subtable (per-position
// coverage, no backtrack/lookahead).
func buildContextFormat3(input [][]uint16, actions []SequenceLookupRecord) []byte {
	var b []byte
	encU16(&b, 3)
	encU16(&b, uint16(len(input)))
	covOffStart := len(b)
	for range input {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(actions)))
	for _, a := range actions {
		encU16(&b, a.SequenceIndex)
		encU16(&b, a.LookupListIndex)
	}
	for i, gids := range input {
		off := uint16(len(b))
		b[covOffStart+2*i] = byte(off >> 8)
		b[covOffStart+2*i+1] = byte(off)
		b = append(b, buildCoverageFormat1(gids)...)
	}
	return b
}

func TestGSUBContextFormat3(t *testing.T) {
	sub := buildContextFormat3(
		[][]uint16{{1}, {2}, {3}},
		[]SequenceLookupRecord{{SequenceIndex: 1, LookupListIndex: 42}},
	)
	data := buildGSUBWithSubtable(5, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	acts, consumed, ok := tbl.MatchContext(0, []uint16{1, 2, 3}, 0)
	if !ok {
		t.Fatal("expected match")
	}
	if consumed != 3 {
		t.Errorf("consumed: got %d, want 3", consumed)
	}
	if len(acts) != 1 || acts[0].LookupListIndex != 42 {
		t.Errorf("actions: got %+v, want [{seq=1, lookup=42}]", acts)
	}
	if _, _, ok := tbl.MatchContext(0, []uint16{1, 2, 4}, 0); ok {
		t.Error("mismatched last glyph should not match")
	}
}

// buildContextFormat1 builds a Type-5 Format-1 subtable with one rule.
func buildContextFormat1(coverageGIDs []uint16, inputTail []uint16, actions []SequenceLookupRecord) []byte {
	// Rule: inputSize = 1 + len(inputTail).
	var rule []byte
	encU16(&rule, uint16(1+len(inputTail)))
	encU16(&rule, uint16(len(actions)))
	for _, g := range inputTail {
		encU16(&rule, g)
	}
	for _, a := range actions {
		encU16(&rule, a.SequenceIndex)
		encU16(&rule, a.LookupListIndex)
	}

	// RuleSet: 1 rule.
	var ruleSet []byte
	encU16(&ruleSet, 1)
	encU16(&ruleSet, 4) // offset to rule from RuleSet start: header(2) + offsets(2) = 4
	ruleSet = append(ruleSet, rule...)

	// Subtable.
	cov := buildCoverageFormat1(coverageGIDs)
	var sub []byte
	encU16(&sub, 1)
	headerLen := 6 + 2*len(coverageGIDs)
	covOff := headerLen
	ruleSetOff := covOff + len(cov)
	encU16(&sub, uint16(covOff))
	encU16(&sub, uint16(len(coverageGIDs)))
	for range coverageGIDs {
		encU16(&sub, uint16(ruleSetOff))
	}
	sub = append(sub, cov...)
	sub = append(sub, ruleSet...)
	return sub
}

func TestGSUBContextFormat1(t *testing.T) {
	// Coverage = {5}; rule input = [5, 6, 7]; action = apply lookup 11 at seq 2.
	sub := buildContextFormat1(
		[]uint16{5},
		[]uint16{6, 7},
		[]SequenceLookupRecord{{SequenceIndex: 2, LookupListIndex: 11}},
	)
	data := buildGSUBWithSubtable(5, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	acts, consumed, ok := tbl.MatchContext(0, []uint16{5, 6, 7}, 0)
	if !ok || consumed != 3 || len(acts) != 1 || acts[0].LookupListIndex != 11 {
		t.Errorf("MatchContext: got (%+v, %d, %v)", acts, consumed, ok)
	}
	if _, _, ok := tbl.MatchContext(0, []uint16{5, 6, 8}, 0); ok {
		t.Error("third glyph mismatch should not match")
	}
}

func TestGSUBReverseChainSingle(t *testing.T) {
	// Rule: replace glyph 2 with glyph 200 when preceded by 1 and followed by 3.
	sub := buildReverseChain(
		[]uint16{2},     // coverage
		[][]uint16{{1}}, // backtrack
		[][]uint16{{3}}, // lookahead
		[]uint16{200},   // substitutes (same length as coverage)
	)
	data := buildGSUBWithSubtable(8, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	out, ok := tbl.ReverseChainSingle(0, []uint16{1, 2, 3}, 1)
	if !ok || out != 200 {
		t.Errorf("ReverseChainSingle: got (%d, %v), want (200, true)", out, ok)
	}
	if _, ok := tbl.ReverseChainSingle(0, []uint16{9, 2, 3}, 1); ok {
		t.Error("backtrack mismatch should not match")
	}
	if _, ok := tbl.ReverseChainSingle(0, []uint16{1, 2, 9}, 1); ok {
		t.Error("lookahead mismatch should not match")
	}
	if _, ok := tbl.ReverseChainSingle(0, []uint16{1, 9, 3}, 1); ok {
		t.Error("coverage mismatch should not match")
	}
}
