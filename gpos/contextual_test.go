// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "testing"

// buildGPOSChainingFormat3 builds a Type-8 Chaining Context Format-3 GPOS
// subtable with the given backtrack/input/lookahead per-position coverage.
func buildGPOSChainingFormat3(
	backtrack, input, lookahead [][]uint16,
	actions []SequenceLookupRecord,
) []byte {
	var b []byte
	encU16(&b, 3)
	encU16(&b, uint16(len(backtrack)))
	backOffStart := len(b)
	for range backtrack {
		encU16(&b, 0)
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

func TestGPOSChainingContextFormat3(t *testing.T) {
	sub := buildGPOSChainingFormat3(
		[][]uint16{{1}},
		[][]uint16{{2}},
		[][]uint16{{3}},
		[]SequenceLookupRecord{{SequenceIndex: 0, LookupListIndex: 7}},
	)
	data := buildGPOSWithSubtable(8, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	acts, consumed, ok := tbl.MatchChainingContext(0, []uint16{1, 2, 3}, 1)
	if !ok || consumed != 1 || len(acts) != 1 || acts[0].LookupListIndex != 7 {
		t.Errorf("MatchChainingContext: got (%+v, %d, %v)", acts, consumed, ok)
	}
	if _, _, ok := tbl.MatchChainingContext(0, []uint16{1, 2, 4}, 1); ok {
		t.Error("lookahead mismatch should not match")
	}
}

// buildGPOSContextFormat3 builds a Type-7 Context Format-3 GPOS subtable.
func buildGPOSContextFormat3(input [][]uint16, actions []SequenceLookupRecord) []byte {
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

func TestGPOSContextFormat3(t *testing.T) {
	sub := buildGPOSContextFormat3(
		[][]uint16{{10}, {20}},
		[]SequenceLookupRecord{{SequenceIndex: 1, LookupListIndex: 3}},
	)
	data := buildGPOSWithSubtable(7, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	acts, consumed, ok := tbl.MatchContext(0, []uint16{10, 20}, 0)
	if !ok || consumed != 2 || len(acts) != 1 || acts[0].LookupListIndex != 3 {
		t.Errorf("MatchContext: got (%+v, %d, %v)", acts, consumed, ok)
	}
	if _, _, ok := tbl.MatchContext(0, []uint16{10, 99}, 0); ok {
		t.Error("second-glyph mismatch should not match")
	}
}
