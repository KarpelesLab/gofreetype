// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

import "testing"

// encU16 / buildCovF1 are small helpers local to these tests so we don't
// depend on the gsub/gpos packages (which would introduce a cycle).
func encU16(b *[]byte, v uint16) { *b = append(*b, byte(v>>8), byte(v)) }

func buildCovF1(gids []uint16) []byte {
	var b []byte
	encU16(&b, 1)
	encU16(&b, uint16(len(gids)))
	for _, g := range gids {
		encU16(&b, g)
	}
	return b
}

// buildContextFormat3Subtable constructs a context Format-3 subtable:
// per-position Coverage tables + action records.
func buildContextFormat3Subtable(input [][]uint16, actions []SequenceLookupRecord) []byte {
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
		b = append(b, buildCovF1(gids)...)
	}
	return b
}

func TestMatchContextSubtableFormat3(t *testing.T) {
	sub := buildContextFormat3Subtable(
		[][]uint16{{1}, {2}, {3}},
		[]SequenceLookupRecord{{SequenceIndex: 1, LookupListIndex: 42}},
	)
	acts, consumed, ok, err := MatchContextSubtable(sub, []uint16{1, 2, 3}, 0)
	if err != nil || !ok {
		t.Fatalf("MatchContextSubtable: ok=%v err=%v", ok, err)
	}
	if consumed != 3 {
		t.Errorf("consumed: got %d, want 3", consumed)
	}
	if len(acts) != 1 || acts[0].LookupListIndex != 42 {
		t.Errorf("actions: got %+v, want [{1, 42}]", acts)
	}
	// Mismatch at second glyph -> no match.
	if _, _, ok, _ := MatchContextSubtable(sub, []uint16{1, 99, 3}, 0); ok {
		t.Error("expected no match with wrong glyph")
	}
}

// buildChainingFormat3Subtable constructs a chaining-context Format-3
// subtable with per-position backtrack / input / lookahead Coverage
// tables.
func buildChainingFormat3Subtable(back, input, ahead [][]uint16, actions []SequenceLookupRecord) []byte {
	var b []byte
	encU16(&b, 3)
	encU16(&b, uint16(len(back)))
	backStart := len(b)
	for range back {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(input)))
	inputStart := len(b)
	for range input {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(ahead)))
	aheadStart := len(b)
	for range ahead {
		encU16(&b, 0)
	}
	encU16(&b, uint16(len(actions)))
	for _, a := range actions {
		encU16(&b, a.SequenceIndex)
		encU16(&b, a.LookupListIndex)
	}
	patch := func(start int, covs [][]uint16) {
		for i, gids := range covs {
			off := uint16(len(b))
			b[start+2*i] = byte(off >> 8)
			b[start+2*i+1] = byte(off)
			b = append(b, buildCovF1(gids)...)
		}
	}
	patch(backStart, back)
	patch(inputStart, input)
	patch(aheadStart, ahead)
	return b
}

func TestMatchChainingContextSubtableFormat3(t *testing.T) {
	sub := buildChainingFormat3Subtable(
		[][]uint16{{1}},
		[][]uint16{{2}},
		[][]uint16{{3}},
		[]SequenceLookupRecord{{SequenceIndex: 0, LookupListIndex: 7}},
	)
	// [1, 2, 3] starting at position 1 — 1 is the backtrack, 2 is input,
	// 3 is lookahead. Match should fire and consume 1 glyph.
	acts, consumed, ok, err := MatchChainingContextSubtable(sub, []uint16{1, 2, 3}, 1)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if consumed != 1 {
		t.Errorf("consumed: got %d, want 1", consumed)
	}
	if len(acts) != 1 || acts[0].LookupListIndex != 7 {
		t.Errorf("acts: got %+v", acts)
	}
	// At position 0 there isn't enough backtrack history.
	if _, _, ok, _ := MatchChainingContextSubtable(sub, []uint16{1, 2, 3}, 0); ok {
		t.Error("should not match at position 0")
	}
}

// TestMatchContextSubtableUnsupportedFormat exercises the unsupported-
// format error path.
func TestMatchContextSubtableUnsupportedFormat(t *testing.T) {
	sub := []byte{0, 99}
	if _, _, _, err := MatchContextSubtable(sub, []uint16{1}, 0); err == nil {
		t.Error("expected error for bogus format")
	}
}

func TestFindFeatureOutOfRange(t *testing.T) {
	tbl := &Table{}
	if tbl.FindFeature(0) != nil {
		t.Error("FindFeature on empty table should return nil")
	}
}

func TestFormatErrorAndUnsupportedError(t *testing.T) {
	if err := FormatError("bad"); err.Error() != "layout: invalid: bad" {
		t.Errorf("FormatError: got %q", err.Error())
	}
	if err := UnsupportedError("nope"); err.Error() != "layout: unsupported: nope" {
		t.Errorf("UnsupportedError: got %q", err.Error())
	}
}
