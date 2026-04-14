// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package cff provides a parser for the Compact Font Format (CFF) table
// used by OpenType/CFF fonts to carry PostScript Type 2 charstrings.
//
// Only CFF v1 is supported here. CFF2 (used by variable fonts) is a
// different binary format and is handled by a separate parser.
package cff

import (
	"fmt"
)

// FormatError reports a malformed CFF table.
type FormatError string

func (e FormatError) Error() string { return "cff: invalid: " + string(e) }

// UnsupportedError reports a CFF feature we do not implement.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "cff: unsupported: " + string(e) }

// Font is a parsed CFF font. A single .otf file usually contains one font
// even though CFF itself supports multi-font collections.
type Font struct {
	// Data holds the raw CFF table bytes. Other slices in this struct are
	// subslices of Data so they share backing storage.
	Data []byte

	// PostScriptName is the font's PostScript name.
	PostScriptName string

	// NumGlyphs is the number of glyphs in CharStrings.
	NumGlyphs int

	// IsCIDKeyed is true if the font is a CID-keyed font. CID fonts have a
	// per-glyph FDSelect that chooses which FDArray entry (and therefore
	// which Private DICT + Local Subrs) a glyph uses for hinting.
	IsCIDKeyed bool

	// FontMatrix is the 6-element font-to-em transform from the Top DICT.
	// Defaults to {0.001, 0, 0, 0.001, 0, 0} when absent.
	FontMatrix [6]float64

	// DefaultWidthX and NominalWidthX come from the Private DICT and affect
	// the charstring `width` operand interpretation.
	DefaultWidthX float64
	NominalWidthX float64

	// CharStrings is the raw Type 2 charstring for each glyph, indexed by
	// glyph id.
	CharStrings [][]byte

	// GlobalSubrs is the shared subroutine table referenced by callgsubr.
	GlobalSubrs [][]byte

	// LocalSubrs is the subroutine table for a non-CID font. For CID fonts
	// this slice is empty; use FDSubrs with FDSelect[gid] to pick the
	// correct per-FD local subrs.
	LocalSubrs [][]byte

	// FDSelect maps glyph id -> FDArray index. Empty for non-CID fonts.
	FDSelect []uint8

	// FDSubrs is the per-FDArray local-subroutine table. Empty for non-CID
	// fonts.
	FDSubrs [][][]byte

	// FDDefaultWidthX / FDNominalWidthX are per-FD width parameters.
	FDDefaultWidthX []float64
	FDNominalWidthX []float64

	// strings holds the CFF String INDEX (indexed by SID - 391).
	strings [][]byte
	// charset maps glyph id -> SID (for SID-keyed fonts) or CID
	// (for CID-keyed fonts).
	charset []uint16
}

// Parse parses a CFF v1 table.
func Parse(data []byte) (*Font, error) {
	if len(data) < 4 {
		return nil, FormatError("table too short")
	}
	major := data[0]
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("CFF major version %d (only 1 is supported)", major))
	}
	hdrSize := int(data[2])
	if hdrSize < 4 || hdrSize > len(data) {
		return nil, FormatError("bad hdrSize")
	}
	// offSize in the header is the absolute-offset width for items outside
	// the INDEXes — not all CFF tables use it; INDEXes embed their own
	// offSize.

	f := &Font{
		Data:       data,
		FontMatrix: [6]float64{0.001, 0, 0, 0.001, 0, 0},
	}

	// Name INDEX.
	nameIndex, off, err := parseIndex(data, hdrSize)
	if err != nil {
		return nil, fmt.Errorf("Name INDEX: %w", err)
	}
	if len(nameIndex) == 0 {
		return nil, FormatError("empty Name INDEX")
	}
	f.PostScriptName = string(nameIndex[0])

	// Top DICT INDEX — one entry per font.
	topDictIndex, off, err := parseIndex(data, off)
	if err != nil {
		return nil, fmt.Errorf("Top DICT INDEX: %w", err)
	}
	if len(topDictIndex) != len(nameIndex) {
		return nil, FormatError("Name INDEX / Top DICT INDEX size mismatch")
	}
	topDict := topDictIndex[0]

	// String INDEX.
	stringIndex, off, err := parseIndex(data, off)
	if err != nil {
		return nil, fmt.Errorf("String INDEX: %w", err)
	}
	f.strings = stringIndex

	// Global Subr INDEX.
	globalSubrs, _, err := parseIndex(data, off)
	if err != nil {
		return nil, fmt.Errorf("Global Subr INDEX: %w", err)
	}
	f.GlobalSubrs = globalSubrs

	// Parse the Top DICT to find per-font offsets.
	td, err := parseTopDict(topDict)
	if err != nil {
		return nil, fmt.Errorf("Top DICT: %w", err)
	}
	if td.hasFontMatrix {
		f.FontMatrix = td.fontMatrix
	}
	f.IsCIDKeyed = td.isCID

	// CharStrings INDEX.
	if td.charStringsOffset == 0 {
		return nil, FormatError("Top DICT has no CharStrings offset")
	}
	if td.charStringType != 2 {
		return nil, UnsupportedError(fmt.Sprintf("CharStringType %d (only type 2 is supported)", td.charStringType))
	}
	charStrings, _, err := parseIndex(data, td.charStringsOffset)
	if err != nil {
		return nil, fmt.Errorf("CharStrings INDEX: %w", err)
	}
	f.CharStrings = charStrings
	f.NumGlyphs = len(charStrings)

	// Charset (per-glyph SID / CID). Optional; absence leaves glyph names
	// unresolvable but doesn't fail the parse.
	if cs, err := parseCharset(data, td.charsetOffset, f.NumGlyphs); err == nil {
		f.charset = cs
	}

	if td.isCID {
		if err := parseCIDFont(f, td); err != nil {
			return nil, err
		}
	} else {
		if err := parseSIDFont(f, td); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func parseSIDFont(f *Font, td *topDict) error {
	if td.privateSize == 0 {
		// No Private DICT — degenerate but not necessarily fatal.
		return nil
	}
	if td.privateOffset+td.privateSize > len(f.Data) {
		return FormatError("Private DICT out of bounds")
	}
	priv := f.Data[td.privateOffset : td.privateOffset+td.privateSize]
	pd, err := parsePrivateDict(priv)
	if err != nil {
		return fmt.Errorf("Private DICT: %w", err)
	}
	f.DefaultWidthX = pd.defaultWidthX
	f.NominalWidthX = pd.nominalWidthX

	if pd.subrsOffset != 0 {
		// subrs offset is relative to the Private DICT start.
		subrsOffAbs := td.privateOffset + pd.subrsOffset
		locals, _, err := parseIndex(f.Data, subrsOffAbs)
		if err != nil {
			return fmt.Errorf("Local Subr INDEX: %w", err)
		}
		f.LocalSubrs = locals
	}
	return nil
}

func parseCIDFont(f *Font, td *topDict) error {
	if td.fdArrayOffset == 0 || td.fdSelectOffset == 0 {
		return FormatError("CID-keyed font missing FDArray or FDSelect")
	}
	fdArrayDicts, _, err := parseIndex(f.Data, td.fdArrayOffset)
	if err != nil {
		return fmt.Errorf("FDArray INDEX: %w", err)
	}
	nFD := len(fdArrayDicts)
	f.FDSubrs = make([][][]byte, nFD)
	f.FDDefaultWidthX = make([]float64, nFD)
	f.FDNominalWidthX = make([]float64, nFD)

	for i, dictBytes := range fdArrayDicts {
		// Each FDArray entry is a Font DICT with a Private (size, offset) pair.
		fd, err := parseTopDict(dictBytes)
		if err != nil {
			return fmt.Errorf("FDArray[%d] DICT: %w", i, err)
		}
		if fd.privateSize == 0 {
			continue
		}
		if fd.privateOffset+fd.privateSize > len(f.Data) {
			return FormatError("FDArray Private DICT out of bounds")
		}
		priv := f.Data[fd.privateOffset : fd.privateOffset+fd.privateSize]
		pd, err := parsePrivateDict(priv)
		if err != nil {
			return fmt.Errorf("FDArray[%d] Private DICT: %w", i, err)
		}
		f.FDDefaultWidthX[i] = pd.defaultWidthX
		f.FDNominalWidthX[i] = pd.nominalWidthX
		if pd.subrsOffset != 0 {
			abs := fd.privateOffset + pd.subrsOffset
			locals, _, err := parseIndex(f.Data, abs)
			if err != nil {
				return fmt.Errorf("FDArray[%d] Local Subrs: %w", i, err)
			}
			f.FDSubrs[i] = locals
		}
	}

	f.FDSelect, err = parseFDSelect(f.Data, td.fdSelectOffset, f.NumGlyphs)
	if err != nil {
		return fmt.Errorf("FDSelect: %w", err)
	}
	return nil
}
