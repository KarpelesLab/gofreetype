// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"encoding/binary"
	"testing"
)

// buildTTC wraps the given SFNT byte slices in a TrueType Collection
// header. Each sub-font's table directory entries are rewritten so their
// offsets point at the new absolute positions inside the TTC (real TTCs
// in the wild share table data across fonts; this helper just relocates
// each font's table offsets, which is enough for round-trip parsing).
func buildTTC(fonts [][]byte) []byte {
	n := len(fonts)
	headerLen := 12 + 4*n
	totalLen := headerLen
	offsets := make([]uint32, n)
	for i, f := range fonts {
		offsets[i] = uint32(totalLen)
		totalLen += len(f)
	}
	out := make([]byte, totalLen)
	copy(out[0:4], "ttcf")
	binary.BigEndian.PutUint32(out[4:8], 0x00010000)
	binary.BigEndian.PutUint32(out[8:12], uint32(n))
	for i, off := range offsets {
		binary.BigEndian.PutUint32(out[12+4*i:], off)
	}
	for i, f := range fonts {
		base := offsets[i]
		copy(out[base:], f)
		// Rewrite table directory offsets so they reflect the font's
		// new absolute position within the TTC.
		numTables := binary.BigEndian.Uint16(f[4:6])
		for j := 0; j < int(numTables); j++ {
			recStart := int(base) + 12 + 16*j
			oldOff := binary.BigEndian.Uint32(out[recStart+8 : recStart+12])
			binary.BigEndian.PutUint32(out[recStart+8:recStart+12], base+oldOff)
		}
	}
	return out
}

func TestNumFontsSingle(t *testing.T) {
	data := buildMinimalSFNT(0x00010000, minimalCFFTables())
	n, err := NumFonts(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("NumFonts(single): got %d, want 1", n)
	}
}

func TestParseIndexSingle(t *testing.T) {
	data := buildMinimalSFNT(0x00010000, minimalCFFTables())
	f, err := ParseIndex(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("ParseIndex returned nil font")
	}
	// Index 1 must fail for a non-collection.
	if _, err := ParseIndex(data, 1); err == nil {
		t.Error("ParseIndex(1) on non-TTC: err=nil, want error")
	}
}

func TestNumFontsAndParseIndexTTC(t *testing.T) {
	f1 := buildMinimalSFNT(0x00010000, minimalCFFTables())
	f2 := buildMinimalSFNT(0x00010000, minimalCFFTables())
	ttc := buildTTC([][]byte{f1, f2})

	n, err := NumFonts(ttc)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("NumFonts(TTC): got %d, want 2", n)
	}

	for i := 0; i < 2; i++ {
		f, err := ParseIndex(ttc, i)
		if err != nil {
			t.Fatalf("ParseIndex(%d): %v", i, err)
		}
		if f.Kind() != FontKindCFF {
			t.Errorf("ParseIndex(%d) Kind: got %d, want FontKindCFF", i, f.Kind())
		}
	}

	// Out-of-range indices must fail.
	if _, err := ParseIndex(ttc, 2); err == nil {
		t.Error("ParseIndex(2) on 2-font TTC: err=nil, want error")
	}
	if _, err := ParseIndex(ttc, -1); err == nil {
		t.Error("ParseIndex(-1): err=nil, want error")
	}
}
