// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"encoding/binary"
	"io/ioutil"
	"testing"
)

// buildMinimalSFNT constructs a minimal valid SFNT file containing the tables
// listed in `tables`. The magic is one of 0x00010000 (TrueType) or 0x4F54544F
// ("OTTO", OpenType/CFF).
func buildMinimalSFNT(magic uint32, tables []struct {
	tag  string
	data []byte
}) []byte {
	n := len(tables)
	headerLen := 12 + 16*n
	// Each table body is 4-byte aligned. Compute absolute offsets.
	offsets := make([]int, n)
	cursor := headerLen
	for i, tb := range tables {
		offsets[i] = cursor
		cursor += len(tb.data)
		// 4-byte padding
		for cursor%4 != 0 {
			cursor++
		}
	}
	buf := make([]byte, cursor)

	binary.BigEndian.PutUint32(buf[0:], magic)
	binary.BigEndian.PutUint16(buf[4:], uint16(n))
	// searchRange / entrySelector / rangeShift left as zero — Parse doesn't check.
	for i, tb := range tables {
		recordOff := 12 + 16*i
		copy(buf[recordOff:recordOff+4], tb.tag)
		// checksum left zero; Parse doesn't verify.
		binary.BigEndian.PutUint32(buf[recordOff+8:], uint32(offsets[i]))
		binary.BigEndian.PutUint32(buf[recordOff+12:], uint32(len(tb.data)))
		copy(buf[offsets[i]:], tb.data)
	}
	return buf
}

// minimalTables returns a set of tables sufficient for parse to succeed for a
// CFF-outlined OpenType font carrying exactly one glyph (.notdef).
func minimalCFFTables() []struct {
	tag  string
	data []byte
} {
	// head: 54 bytes. Only fUnitsPerEm at [18:20], bounds at [36:44], and
	// indexToLocFormat at [50:52] are read.
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:], 1000) // fUnitsPerEm
	// Give a generous bounding box so face/raster allocates a mask large
	// enough for test glyphs: xMin=-200, yMin=-200, xMax=1000, yMax=1000.
	var xMin, yMin int16 = -200, -200
	binary.BigEndian.PutUint16(head[36:], uint16(xMin))
	binary.BigEndian.PutUint16(head[38:], uint16(yMin))
	binary.BigEndian.PutUint16(head[40:], 1000)
	binary.BigEndian.PutUint16(head[42:], 1000)

	// maxp v0.5 = 6 bytes: version (0x00005000) + numGlyphs (1).
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:], 0x00005000)
	binary.BigEndian.PutUint16(maxp[4:], 1)

	// hhea: 36 bytes. We only care about offsets 4,6,8,18,20,34.
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:], 800)    // ascent
	binary.BigEndian.PutUint16(hhea[6:], 0xff38) // descent = -200
	binary.BigEndian.PutUint16(hhea[8:], 100)    // lineGap
	binary.BigEndian.PutUint16(hhea[18:], 1)     // caretSlopeRise
	binary.BigEndian.PutUint16(hhea[34:], 1)     // numberOfHMetrics

	// hmtx: 4*nHMetric + 2*(nGlyph-nHMetric) = 4 bytes for nHMetric=1, nGlyph=1.
	hmtx := []byte{0x02, 0x00, 0x00, 0x00} // advance=512, lsb=0

	// cmap: one format-0 subtable mapping everything to glyph 0.
	var fmt0 [262]byte
	binary.BigEndian.PutUint16(fmt0[0:], 0)
	binary.BigEndian.PutUint16(fmt0[2:], 262)
	// language = 0, glyph ids all zero.
	cmap := append([]byte{
		0, 0, // version
		0, 1, // numSubtables
		0, 0, 0, 3, // PID=0 PSID=3 (Unicode BMP)
		0, 0, 0, 12, // offset to subtable
	}, fmt0[:]...)

	// CFF: a minimal valid CFF v1 with one glyph whose charstring is just
	// `endchar` (op 14).
	cff := buildMinimalCFF()

	return []struct {
		tag  string
		data []byte
	}{
		{"CFF ", cff},
		{"cmap", cmap},
		{"head", head},
		{"hhea", hhea},
		{"hmtx", hmtx},
		{"maxp", maxp},
	}
}

// buildMinimalCFF constructs a valid CFF v1 table with one glyph whose
// charstring is a bare `endchar`. Used by Parse tests so we don't have to
// import cff-test-only helpers.
func buildMinimalCFF() []byte {
	encIdx := func(objs [][]byte) []byte {
		count := len(objs)
		if count == 0 {
			return []byte{0, 0}
		}
		offsets := []int{1}
		for _, o := range objs {
			offsets = append(offsets, offsets[len(offsets)-1]+len(o))
		}
		offSize := 1
		if offsets[count] > 255 {
			offSize = 2
		}
		buf := []byte{byte(count >> 8), byte(count), byte(offSize)}
		for _, o := range offsets {
			for s := offSize - 1; s >= 0; s-- {
				buf = append(buf, byte(o>>(8*s)))
			}
		}
		for _, o := range objs {
			buf = append(buf, o...)
		}
		return buf
	}
	// DICT int encoding for small non-negative values (0..107) is a single byte.
	encInt := func(v int) []byte {
		if v >= -107 && v <= 107 {
			return []byte{byte(v + 139)}
		}
		// 28 short-int form: 3 bytes.
		return []byte{28, byte(v >> 8), byte(v)}
	}

	empty := encIdx(nil)
	nameIdx := encIdx([][]byte{[]byte("T")})
	charStrings := encIdx([][]byte{{14}}) // endchar only

	// Layout: header(4) + nameIdx + topIdx + strings(empty) + globalSubrs(empty) + charStrings + priv + localSubrs(empty).
	// Iterate to stabilize offsets since they're embedded in the Top DICT.
	priv := func(subrsOff int) []byte {
		var b []byte
		b = append(b, encInt(subrsOff)...)
		b = append(b, 19) // Subrs
		return b
	}
	privSize := len(priv(0))
	pv := priv(privSize)

	var topDict []byte
	encTop := func(csOff, privOff int) []byte {
		var b []byte
		b = append(b, encInt(csOff)...)
		b = append(b, 17) // CharStrings
		b = append(b, encInt(privSize)...)
		b = append(b, encInt(privOff)...)
		b = append(b, 18) // Private
		return b
	}
	csOff, privOff := 0, 0
	for iter := 0; iter < 8; iter++ {
		topDict = encTop(csOff, privOff)
		topIdx := encIdx([][]byte{topDict})
		hdr := 4
		nameOff := hdr
		topOff := nameOff + len(nameIdx)
		stringsOff := topOff + len(topIdx)
		globalSubrsOff := stringsOff + len(empty)
		csNew := globalSubrsOff + len(empty)
		privNew := csNew + len(charStrings)
		if csNew == csOff && privNew == privOff {
			break
		}
		csOff, privOff = csNew, privNew
	}
	topIdx := encIdx([][]byte{topDict})

	var out []byte
	out = append(out, 1, 0, 4, 2) // header
	out = append(out, nameIdx...)
	out = append(out, topIdx...)
	out = append(out, empty...) // String INDEX
	out = append(out, empty...) // Global Subrs INDEX
	out = append(out, charStrings...)
	out = append(out, pv...)
	out = append(out, empty...) // Local Subrs INDEX
	return out
}

func TestParseOpenTypeCFF(t *testing.T) {
	data := buildMinimalSFNT(0x4F54544F, minimalCFFTables())
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(synthetic OTF): %v", err)
	}
	if got, want := f.Kind(), FontKindCFF; got != want {
		t.Errorf("Kind: got %d, want %d", got, want)
	}
	if len(f.cff) == 0 {
		t.Error("expected f.cff to be populated from the CFF table")
	}
	if len(f.glyf) != 0 {
		t.Error("expected f.glyf to be empty for an OTF font")
	}
	// GlyphBuf.Load for a CFF glyph whose charstring is only `endchar`
	// should succeed and produce no segments.
	g := &GlyphBuf{}
	if err := g.Load(f, 64, 0, 0); err != nil {
		t.Errorf("GlyphBuf.Load on CFF: %v", err)
	}
	if len(g.Segments) != 0 {
		t.Errorf("Segments: got %d, want 0", len(g.Segments))
	}
	if len(g.Points) != 0 {
		t.Errorf("Points: got %d, want 0 for CFF", len(g.Points))
	}
}

func TestParseTrueTypeKind(t *testing.T) {
	data, err := ioutil.ReadFile("../testdata/luxisr.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Kind(), FontKindTrueType; got != want {
		t.Errorf("Kind(luxisr.ttf): got %d, want %d", got, want)
	}
	if len(f.cff) != 0 {
		t.Error("f.cff should be empty for a TTF")
	}
}

func TestParseOpenTypeCFF2(t *testing.T) {
	tables := minimalCFFTables()
	// Swap "CFF " for "CFF2".
	for i := range tables {
		if tables[i].tag == "CFF " {
			tables[i].tag = "CFF2"
		}
	}
	data := buildMinimalSFNT(0x4F54544F, tables)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(synthetic CFF2): %v", err)
	}
	if got, want := f.Kind(), FontKindCFF2; got != want {
		t.Errorf("Kind: got %d, want %d", got, want)
	}
}
