// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package gdef parses the OpenType GDEF (Glyph Definition) table. The
// information here is consumed by GSUB and GPOS lookups to decide which
// glyphs to skip (e.g. marks when working at the base-glyph level), which
// marks belong to which attachment class, and which marks are in named
// filter sets.
package gdef

import (
	"fmt"

	"github.com/KarpelesLab/gofreetype/layout"
)

// Well-known glyph classes from GDEF.
const (
	ClassBase      = 1
	ClassLigature  = 2
	ClassMark      = 3
	ClassComponent = 4
)

// FormatError reports a malformed GDEF table.
type FormatError string

func (e FormatError) Error() string { return "gdef: invalid: " + string(e) }

// UnsupportedError reports a GDEF feature we do not implement.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "gdef: unsupported: " + string(e) }

// Table is a parsed GDEF table.
type Table struct {
	Data               []byte
	Major, Minor       uint16
	GlyphClassDef      *layout.ClassDef
	MarkAttachClassDef *layout.ClassDef
	MarkGlyphSets      []*layout.Coverage
}

// Parse decodes a GDEF table.
func Parse(data []byte) (*Table, error) {
	if len(data) < 12 {
		return nil, FormatError("table too short")
	}
	t := &Table{
		Data:  data,
		Major: u16(data, 0),
		Minor: u16(data, 2),
	}
	if t.Major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("GDEF major version %d", t.Major))
	}

	if off := int(u16(data, 4)); off != 0 {
		cd, err := layout.ParseClassDef(data, off)
		if err != nil {
			return nil, fmt.Errorf("glyphClassDef: %w", err)
		}
		t.GlyphClassDef = cd
	}
	// offset 6 = AttachList (skipped)
	// offset 8 = LigCaretList (skipped)
	if off := int(u16(data, 10)); off != 0 {
		cd, err := layout.ParseClassDef(data, off)
		if err != nil {
			return nil, fmt.Errorf("markAttachClassDef: %w", err)
		}
		t.MarkAttachClassDef = cd
	}

	// v1.2 adds MarkGlyphSetsDef.
	if t.Minor >= 2 {
		if len(data) < 14 {
			return nil, FormatError("GDEF v1.2 header truncated")
		}
		if mgsOff := int(u16(data, 12)); mgsOff != 0 {
			if err := t.parseMarkGlyphSets(mgsOff); err != nil {
				return nil, fmt.Errorf("markGlyphSetsDef: %w", err)
			}
		}
	}
	return t, nil
}

func (t *Table) parseMarkGlyphSets(off int) error {
	if off+4 > len(t.Data) {
		return FormatError("MarkGlyphSetsDef header truncated")
	}
	// uint16 format (1)
	// uint16 markGlyphSetCount
	// Offset32 coverageOffsets[markGlyphSetCount] — relative to MarkGlyphSetsDef start
	format := u16(t.Data, off)
	if format != 1 {
		return UnsupportedError(fmt.Sprintf("MarkGlyphSetsDef format %d", format))
	}
	n := int(u16(t.Data, off+2))
	if off+4+4*n > len(t.Data) {
		return FormatError("MarkGlyphSetsDef coverage offsets truncated")
	}
	t.MarkGlyphSets = make([]*layout.Coverage, n)
	for i := 0; i < n; i++ {
		covOff := off + int(u32(t.Data, off+4+4*i))
		cov, err := layout.ParseCoverage(t.Data, covOff)
		if err != nil {
			return fmt.Errorf("markGlyphSets[%d]: %w", i, err)
		}
		t.MarkGlyphSets[i] = cov
	}
	return nil
}

// Class returns the GDEF class of a glyph, or 0 if the glyph isn't classified.
// A return of ClassMark means the glyph is a combining mark.
func (t *Table) Class(g uint16) uint16 {
	return t.GlyphClassDef.Class(g)
}

// MarkAttachClass returns the mark-attachment class of g, or 0 if g has none.
func (t *Table) MarkAttachClass(g uint16) uint16 {
	return t.MarkAttachClassDef.Class(g)
}

// IsMarkInSet reports whether g is a member of the nth mark glyph set.
func (t *Table) IsMarkInSet(setIndex int, g uint16) bool {
	if setIndex < 0 || setIndex >= len(t.MarkGlyphSets) {
		return false
	}
	return t.MarkGlyphSets[setIndex].Index(g) >= 0
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}
