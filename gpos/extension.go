// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

// A GPOS Lookup of type 9 is a thin wrapper that names another lookup type
// and points at its subtable with a 32-bit offset. This lets very large
// lookup subtables live beyond the 64 KiB ceiling imposed by 16-bit
// offsets elsewhere in the table. The wire format is:
//
//	uint16 format (1)
//	uint16 extensionLookupType  (e.g. 2 for pair, 4 for mark-to-base)
//	Offset32 extensionOffset    (from the Extension subtable start)
//
// resolveExtension unwraps extension subtables so callers can work with a
// consistent (type, subtables) pair regardless of whether the lookup was
// stored inline or via an Extension indirection.
func resolveExtension(lookupType uint16, subtables [][]byte) (uint16, [][]byte) {
	if lookupType != 9 {
		return lookupType, subtables
	}
	innerType := uint16(0)
	out := make([][]byte, 0, len(subtables))
	for _, sub := range subtables {
		if len(sub) < 8 {
			continue
		}
		if u16(sub, 0) != 1 {
			// Unknown extension format; skip.
			continue
		}
		inner := u16(sub, 2)
		off := int(u32(sub, 4))
		if inner == 9 {
			// Extensions cannot chain per the spec; be defensive.
			continue
		}
		if off < 0 || off >= len(sub) {
			continue
		}
		if innerType == 0 {
			innerType = inner
		} else if innerType != inner {
			// A single Lookup must have subtables of the same effective type.
			// If the font violates that, surface the first one.
			continue
		}
		out = append(out, sub[off:])
	}
	if innerType == 0 || len(out) == 0 {
		return 0, nil
	}
	return innerType, out
}
