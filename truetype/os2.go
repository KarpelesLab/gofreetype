// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

// OS2Info holds the OS/2 table fields apps commonly care about: the
// standard 9-step weight/width classes, the style flags (regular, bold,
// italic, etc.), and the typographic and usWin ascent/descent metrics
// that differ from hhea's ascent/descent.
type OS2Info struct {
	// Version of the OS/2 table (0..5).
	Version uint16

	// WeightClass is a value 1..1000; conventional anchors are:
	//   100 Thin, 200 Extra-Light, 300 Light, 400 Regular, 500 Medium,
	//   600 Semi-Bold, 700 Bold, 800 Extra-Bold, 900 Black.
	WeightClass uint16

	// WidthClass is 1..9 with anchor 5 = Medium (normal).
	WidthClass uint16

	// FsType is the OpenType embedding permission bits.
	FsType uint16

	// Selection is the fsSelection bitfield.
	Selection uint16

	// TypoAscender / TypoDescender / TypoLineGap are the OpenType-
	// recommended metrics for line layout. They usually match hhea but
	// some fonts diverge.
	TypoAscender  int16
	TypoDescender int16
	TypoLineGap   int16

	// WinAscent / WinDescent are the Windows-clipping metrics. Glyphs
	// outside this box are liable to be clipped on legacy Windows.
	WinAscent  uint16
	WinDescent uint16

	// Panose is the 10-byte PANOSE classification. Rarely reliable but
	// occasionally useful for font matching heuristics.
	Panose [10]byte

	// VendorID is the 4-character ASCII "achVendID" (e.g. "ADBE" for
	// Adobe, "MS  " for Microsoft).
	VendorID [4]byte
}

// Style flags from fsSelection (OS/2 spec).
const (
	StyleItalic    uint16 = 1 << 0
	StyleUnderscore uint16 = 1 << 1
	StyleNegative  uint16 = 1 << 2
	StyleOutlined  uint16 = 1 << 3
	StyleStrikeout uint16 = 1 << 4
	StyleBold      uint16 = 1 << 5
	StyleRegular   uint16 = 1 << 6
	StyleUseTypo   uint16 = 1 << 7
	StyleOblique   uint16 = 1 << 9
)

// parseOS2Info decodes the OS/2 table into a richer OS2Info record.
// Layouts evolved across versions; we read what each version promises
// and leave the rest at zero.
func (f *Font) parseOS2Info() {
	if len(f.os2) < 78 {
		return
	}
	info := &OS2Info{
		Version:     u16(f.os2, 0),
		WeightClass: u16(f.os2, 4),
		WidthClass:  u16(f.os2, 6),
		FsType:      u16(f.os2, 8),
	}
	copy(info.Panose[:], f.os2[32:42])
	copy(info.VendorID[:], f.os2[58:62])
	info.Selection = u16(f.os2, 62)

	// TypoAscender/Descender/LineGap at 68/70/72; WinAscent/Descent 74/76.
	if len(f.os2) >= 78 {
		info.TypoAscender = int16(u16(f.os2, 68))
		info.TypoDescender = int16(u16(f.os2, 70))
		info.TypoLineGap = int16(u16(f.os2, 72))
		info.WinAscent = u16(f.os2, 74)
		info.WinDescent = u16(f.os2, 76)
	}
	f.os2Info = info
}

// OS2 returns the parsed OS/2 info, or nil if the font has no OS/2
// table.
func (f *Font) OS2() *OS2Info {
	return f.os2Info
}

// IsBold reports whether the font is marked bold (either via the
// fsSelection StyleBold bit or a weight class >= 700).
func (f *Font) IsBold() bool {
	if f.os2Info == nil {
		return false
	}
	return f.os2Info.Selection&StyleBold != 0 || f.os2Info.WeightClass >= 700
}

// IsItalic reports whether the font is marked italic or oblique.
func (f *Font) IsItalic() bool {
	if f.os2Info == nil {
		return false
	}
	return f.os2Info.Selection&(StyleItalic|StyleOblique) != 0
}
