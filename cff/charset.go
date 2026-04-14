// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "fmt"

// Predefined CFF charsets. Charset offset values 0, 1, 2 are reserved to
// refer to these standard tables without on-disk data. We only surface
// their names via GlyphName when appropriate.
const (
	predefinedCharsetISOAdobe  = 0 // .notdef + ISOAdobeCharset (ISO-Adobe standard set)
	predefinedCharsetExpert    = 1
	predefinedCharsetExpSubset = 2
)

// parseCharset decodes the per-glyph SID (or CID for CID-keyed fonts)
// array from a CFF charset table. The returned slice has length
// NumGlyphs, with entry [0] implicitly 0 (.notdef).
//
// Three formats are defined:
//
//	Format 0: one uint16 SID per glyph (after glyph 0).
//	Format 1: array of (uint16 first, uint8 nLeft) ranges.
//	Format 2: array of (uint16 first, uint16 nLeft) ranges.
func parseCharset(data []byte, offset, numGlyphs int) ([]uint16, error) {
	if numGlyphs <= 0 {
		return nil, nil
	}
	// Handle predefined charsets (offset 0, 1, 2).
	if offset == predefinedCharsetISOAdobe {
		return predefinedISOAdobeCharset(numGlyphs), nil
	}
	if offset == predefinedCharsetExpert || offset == predefinedCharsetExpSubset {
		// These are expert encodings; return a zeroed slice (all .notdef)
		// rather than fail. Callers can still render; glyph names simply
		// won't resolve.
		return make([]uint16, numGlyphs), nil
	}
	if offset < 0 || offset >= len(data) {
		return nil, FormatError(fmt.Sprintf("charset offset %d out of range", offset))
	}
	sids := make([]uint16, numGlyphs)
	// Glyph 0 is always .notdef (SID 0), regardless of format.
	sids[0] = 0

	format := data[offset]
	p := offset + 1
	switch format {
	case 0:
		if p+2*(numGlyphs-1) > len(data) {
			return nil, FormatError("charset format 0 truncated")
		}
		for i := 1; i < numGlyphs; i++ {
			sids[i] = u16(data, p+2*(i-1))
		}
	case 1:
		i := 1
		for i < numGlyphs {
			if p+3 > len(data) {
				return nil, FormatError("charset format 1 truncated")
			}
			first := u16(data, p)
			nLeft := int(data[p+2])
			for j := 0; j <= nLeft && i < numGlyphs; j++ {
				sids[i] = first + uint16(j)
				i++
			}
			p += 3
		}
	case 2:
		i := 1
		for i < numGlyphs {
			if p+4 > len(data) {
				return nil, FormatError("charset format 2 truncated")
			}
			first := u16(data, p)
			nLeft := int(u16(data, p+2))
			for j := 0; j <= nLeft && i < numGlyphs; j++ {
				sids[i] = first + uint16(j)
				i++
			}
			p += 4
		}
	default:
		return nil, UnsupportedError(fmt.Sprintf("charset format %d", format))
	}
	return sids, nil
}

// GlyphName returns the PostScript name of glyph gid, or "" when the
// font has no charset, gid is out of range, or the SID resolves to a
// non-standard name that isn't in the Strings INDEX.
//
// For CID-keyed fonts, the charset entry is a CID rather than a SID;
// GlyphName returns "cid<N>" in that case so callers at least get a
// unique non-empty identifier.
func (f *Font) GlyphName(gid int) string {
	if gid < 0 || gid >= len(f.charset) {
		return ""
	}
	sid := f.charset[gid]
	if f.IsCIDKeyed {
		return fmt.Sprintf("cid%05d", sid)
	}
	// SID 0..390 are the predefined standard strings.
	if int(sid) < len(standardStrings) {
		return standardStrings[sid]
	}
	customIdx := int(sid) - len(standardStrings)
	if customIdx >= 0 && customIdx < len(f.strings) {
		return string(f.strings[customIdx])
	}
	return ""
}

// predefinedISOAdobeCharset returns SIDs for the first numGlyphs entries
// of the ISO-Adobe standard charset (glyph id -> SID identity for
// glyphs 0..228).
func predefinedISOAdobeCharset(numGlyphs int) []uint16 {
	sids := make([]uint16, numGlyphs)
	for i := 0; i < numGlyphs; i++ {
		sids[i] = uint16(i)
	}
	return sids
}
