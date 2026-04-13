// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// SequenceLookupRecord re-exports the layout type for caller convenience.
type SequenceLookupRecord = layout.SequenceLookupRecord

// MatchContext tries to match a GPOS Type-7 Context rule at position
// `start` in `glyphs`. Returns the nested positioning-lookup actions and
// the number of input glyphs consumed on match.
func (t *Table) MatchContext(lookupIndex uint16, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 7 {
		return nil, 0, false
	}
	for _, sub := range subtables {
		acts, n, found, err := layout.MatchContextSubtable(sub, glyphs, start)
		if err != nil || !found {
			continue
		}
		return acts, n, true
	}
	return nil, 0, false
}

// MatchChainingContext tries to match a GPOS Type-8 Chaining Context rule
// at position `start`. Backtrack and lookahead only constrain context;
// they do not consume input glyphs.
func (t *Table) MatchChainingContext(lookupIndex uint16, glyphs []uint16, start int) ([]SequenceLookupRecord, int, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 8 {
		return nil, 0, false
	}
	for _, sub := range subtables {
		acts, n, found, err := layout.MatchChainingContextSubtable(sub, glyphs, start)
		if err != nil || !found {
			continue
		}
		return acts, n, true
	}
	return nil, 0, false
}
