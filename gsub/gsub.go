// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package gsub implements glyph substitution lookups from an OpenType GSUB
// table. Each Lookup Type lives in its own file; gsub.Table resolves
// extension indirections and dispatches on lookup type.
package gsub

import (
	"github.com/KarpelesLab/gofreetype/layout"
)

// FormatError reports a malformed GSUB structure.
type FormatError string

func (e FormatError) Error() string { return "gsub: invalid: " + string(e) }

// UnsupportedError reports a GSUB feature we do not implement.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "gsub: unsupported: " + string(e) }

// Table is a parsed GSUB table on top of the shared layout structures.
type Table struct {
	*layout.Table
}

// Parse decodes a GSUB table.
func Parse(data []byte) (*Table, error) {
	lt, err := layout.Parse(data)
	if err != nil {
		return nil, err
	}
	return &Table{Table: lt}, nil
}

// resolveExtension unwraps GSUB Type-7 Extension subtables. Its shape is
// identical to GPOS Type-9 Extension: the table names an inner lookup
// type plus a 32-bit offset to the real subtable.
func resolveExtension(lookupType uint16, subtables [][]byte) (uint16, [][]byte) {
	if lookupType != 7 {
		return lookupType, subtables
	}
	innerType := uint16(0)
	out := make([][]byte, 0, len(subtables))
	for _, sub := range subtables {
		if len(sub) < 8 {
			continue
		}
		if u16(sub, 0) != 1 {
			continue
		}
		inner := u16(sub, 2)
		off := int(u32(sub, 4))
		if inner == 7 || off < 0 || off >= len(sub) {
			continue
		}
		if innerType == 0 {
			innerType = inner
		} else if innerType != inner {
			continue
		}
		out = append(out, sub[off:])
	}
	if innerType == 0 || len(out) == 0 {
		return 0, nil
	}
	return innerType, out
}

func u16(b []byte, i int) uint16 {
	if i+1 >= len(b) {
		return 0
	}
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	if i+3 >= len(b) {
		return 0
	}
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}

func intToStr(n int) string {
	// Small non-strconv helper to avoid the import in error messages.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = '0' + byte(n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
