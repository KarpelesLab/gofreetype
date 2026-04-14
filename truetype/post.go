// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import "fmt"

// post table versions we recognize.
const (
	postVersion10 uint32 = 0x00010000
	postVersion20 uint32 = 0x00020000
	postVersion25 uint32 = 0x00025000 // deprecated
	postVersion30 uint32 = 0x00030000
	postVersion40 uint32 = 0x00040000
)

// macStandardGlyphNames is the 258-entry list of glyph names referenced
// by post table version 1.0 and 2.0 (indices 0..257 are fixed).
var macStandardGlyphNames = [...]string{
	".notdef", ".null", "nonmarkingreturn", "space", "exclam",
	"quotedbl", "numbersign", "dollar", "percent", "ampersand",
	"quotesingle", "parenleft", "parenright", "asterisk", "plus",
	"comma", "hyphen", "period", "slash", "zero",
	"one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "colon",
	"semicolon", "less", "equal", "greater", "question",
	"at", "A", "B", "C", "D",
	"E", "F", "G", "H", "I",
	"J", "K", "L", "M", "N",
	"O", "P", "Q", "R", "S",
	"T", "U", "V", "W", "X",
	"Y", "Z", "bracketleft", "backslash", "bracketright",
	"asciicircum", "underscore", "grave", "a", "b",
	"c", "d", "e", "f", "g",
	"h", "i", "j", "k", "l",
	"m", "n", "o", "p", "q",
	"r", "s", "t", "u", "v",
	"w", "x", "y", "z", "braceleft",
	"bar", "braceright", "asciitilde", "Adieresis", "Aring",
	"Ccedilla", "Eacute", "Ntilde", "Odieresis", "Udieresis",
	"aacute", "agrave", "acircumflex", "adieresis", "atilde",
	"aring", "ccedilla", "eacute", "egrave", "ecircumflex",
	"edieresis", "iacute", "igrave", "icircumflex", "idieresis",
	"ntilde", "oacute", "ograve", "ocircumflex", "odieresis",
	"otilde", "uacute", "ugrave", "ucircumflex", "udieresis",
	"dagger", "degree", "cent", "sterling", "section",
	"bullet", "paragraph", "germandbls", "registered", "copyright",
	"trademark", "acute", "dieresis", "notequal", "AE",
	"Oslash", "infinity", "plusminus", "lessequal", "greaterequal",
	"yen", "mu", "partialdiff", "summation", "product",
	"pi", "integral", "ordfeminine", "ordmasculine", "Omega",
	"ae", "oslash", "questiondown", "exclamdown", "logicalnot",
	"radical", "florin", "approxequal", "Delta", "guillemotleft",
	"guillemotright", "ellipsis", "nonbreakingspace", "Agrave", "Atilde",
	"Otilde", "OE", "oe", "endash", "emdash",
	"quotedblleft", "quotedblright", "quoteleft", "quoteright", "divide",
	"lozenge", "ydieresis", "Ydieresis", "fraction", "currency",
	"guilsinglleft", "guilsinglright", "fi", "fl", "daggerdbl",
	"periodcentered", "quotesinglbase", "quotedblbase", "perthousand", "Acircumflex",
	"Ecircumflex", "Aacute", "Edieresis", "Egrave", "Iacute",
	"Icircumflex", "Idieresis", "Eth", "eth", "Yacute",
	"yacute", "Thorn", "thorn", "minus", "multiply",
	"onesuperior", "twosuperior", "threesuperior", "onehalf", "onequarter",
	"threequarters", "franc", "Gbreve", "gbreve", "Idotaccent",
	"Scedilla", "scedilla", "Cacute", "cacute", "Ccaron",
	"ccaron", "dcroat",
}

// PostInfo holds the font-wide values from the post table.
type PostInfo struct {
	// Version is the post table major/minor as 0x00010000 / 0x00020000 etc.
	Version uint32
	// ItalicAngle is the counterclockwise italic angle in degrees (0 for
	// upright fonts; typically -10..-15 for italics).
	ItalicAngle float64
	// UnderlinePosition is the top of the underline relative to baseline,
	// in FUnits (typically negative).
	UnderlinePosition int16
	// UnderlineThickness is the thickness of the underline in FUnits.
	UnderlineThickness int16
	// IsFixedPitch is true when the font is monospaced.
	IsFixedPitch bool

	// glyphNames[i] is the PostScript name for glyph id i.
	// For version 3.0 fonts this is empty (no names).
	glyphNames []string
}

// parsePost decodes the post table and populates f.postInfo. Absent or
// malformed post tables are tolerated by leaving f.postInfo nil.
func (f *Font) parsePost(data []byte) error {
	if len(data) < 32 {
		return nil
	}
	info := &PostInfo{
		Version:            u32(data, 0),
		ItalicAngle:        float64(int32(u32(data, 4))) / 65536.0,
		UnderlinePosition:  int16(u16(data, 8)),
		UnderlineThickness: int16(u16(data, 10)),
		IsFixedPitch:       u32(data, 12) != 0,
	}

	switch info.Version {
	case postVersion10:
		// Standard Mac glyph names for all 258 glyphs.
		if f.nGlyph > 258 {
			return nil // out-of-range, leave glyphNames empty
		}
		info.glyphNames = macStandardGlyphNames[:f.nGlyph]

	case postVersion20:
		// numGlyphs + glyphNameIndex[numGlyphs] + custom Pascal strings.
		if len(data) < 34 {
			return nil
		}
		n := int(u16(data, 32))
		if n != f.nGlyph {
			// Some fonts lie about this. Fall back to using whatever n says.
		}
		indexStart := 34
		if indexStart+2*n > len(data) {
			return nil
		}
		indexes := make([]uint16, n)
		for i := 0; i < n; i++ {
			indexes[i] = u16(data, indexStart+2*i)
		}
		customStart := indexStart + 2*n
		customNames, err := parsePascalStrings(data[customStart:])
		if err != nil {
			return nil
		}
		names := make([]string, n)
		for i, idx := range indexes {
			if int(idx) < len(macStandardGlyphNames) {
				names[i] = macStandardGlyphNames[idx]
			} else {
				cIdx := int(idx) - 258
				if cIdx >= 0 && cIdx < len(customNames) {
					names[i] = customNames[cIdx]
				}
			}
		}
		info.glyphNames = names

	case postVersion30:
		// No glyph names — common in OpenType/CFF fonts where names live
		// in the CFF table's charset instead.

	case postVersion25, postVersion40:
		// Deprecated / rarely used; skip name extraction.
	}

	f.postInfo = info
	return nil
}

// parsePascalStrings decodes a sequence of Pascal-style strings (1-byte
// length prefix, followed by the bytes). The sequence ends at the end
// of the buffer.
func parsePascalStrings(data []byte) ([]string, error) {
	var names []string
	i := 0
	for i < len(data) {
		n := int(data[i])
		i++
		if i+n > len(data) {
			return nil, fmt.Errorf("pascal string runs past buffer")
		}
		names = append(names, string(data[i:i+n]))
		i += n
	}
	return names, nil
}

// GlyphName returns the PostScript name of glyph i, or the empty string
// when the font has no post table, no v1/v2 post table, or no name for
// that glyph.
func (f *Font) GlyphName(i Index) string {
	if f.postInfo == nil {
		return ""
	}
	if int(i) < 0 || int(i) >= len(f.postInfo.glyphNames) {
		return ""
	}
	return f.postInfo.glyphNames[i]
}

// Post returns the parsed post table info, or nil if absent.
func (f *Font) Post() *PostInfo { return f.postInfo }
