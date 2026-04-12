// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import (
	"bytes"
	"testing"
)

// encodeIndex builds a CFF INDEX whose object data is the concatenation of
// objs. Offsets are chosen to be the minimum offSize that fits.
func encodeIndex(objs [][]byte) []byte {
	count := len(objs)
	if count == 0 {
		return []byte{0, 0}
	}
	// Compute offsets (1-based into the data area).
	offsets := make([]int, count+1)
	offsets[0] = 1
	for i, o := range objs {
		offsets[i+1] = offsets[i] + len(o)
	}
	max := offsets[count]
	offSize := 1
	switch {
	case max > 1<<24:
		offSize = 4
	case max > 1<<16:
		offSize = 3
	case max > 1<<8:
		offSize = 2
	}
	var buf bytes.Buffer
	buf.WriteByte(byte(count >> 8))
	buf.WriteByte(byte(count))
	buf.WriteByte(byte(offSize))
	for _, o := range offsets {
		for s := offSize - 1; s >= 0; s-- {
			buf.WriteByte(byte(o >> (8 * s)))
		}
	}
	for _, o := range objs {
		buf.Write(o)
	}
	return buf.Bytes()
}

// encodeInt returns a DICT integer operand encoding for v.
func encodeInt(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		w := v - 108
		return []byte{byte(w>>8) + 247, byte(w)}
	case v >= -1131 && v <= -108:
		w := -v - 108
		return []byte{byte(w>>8) + 251, byte(w)}
	case v >= -32768 && v <= 32767:
		return []byte{28, byte(v >> 8), byte(v)}
	}
	return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

// encodeOp returns a DICT operator byte sequence.
func encodeOp(op uint16) []byte {
	if op > 0xff {
		return []byte{byte(op >> 8), byte(op)}
	}
	return []byte{byte(op)}
}

// TestParseMinimalCFF builds a hand-rolled CFF v1 table with one glyph
// (charstring body is arbitrary bytes — parsing only, no interpretation
// here). It verifies that Parse extracts the PostScript name, charstring,
// and Private DICT fields.
func TestParseMinimalCFF(t *testing.T) {
	// Empty INDEXes used as filler.
	empty := encodeIndex(nil)

	// Name INDEX: one entry with the PostScript name.
	nameIdx := encodeIndex([][]byte{[]byte("MyTestFont")})

	// Placeholders. We will build Top DICT last because it embeds absolute
	// offsets into the file.
	// Layout: header(4) + nameIdx + topDictIdxPlaceholder + stringIdx +
	//         globalSubrsIdx + charStringsIdx + privateDict + localSubrsIdx.

	// CharStrings INDEX: one entry with a trivial endchar (op 14).
	charStrings := encodeIndex([][]byte{{14}})

	// Private DICT: defaultWidthX=500, nominalWidthX=600, Subrs offset points
	// to an empty local subrs INDEX. We'll fix up the Subrs offset after we
	// know where the local subrs INDEX starts.
	// The subrs offset is encoded as the last operand of op 19, relative to
	// the start of the Private DICT. We'll encode with a 2-byte int placeholder
	// and overwrite before finalizing the layout.
	privateDictWithoutSubrs := func(subrsOffset int) []byte {
		var b bytes.Buffer
		b.Write(encodeInt(500))
		b.Write(encodeOp(20)) // defaultWidthX
		b.Write(encodeInt(600))
		b.Write(encodeOp(21)) // nominalWidthX
		b.Write(encodeInt(subrsOffset))
		b.Write(encodeOp(19)) // Subrs
		return b.Bytes()
	}

	localSubrs := encodeIndex(nil)

	// We need to know the size of the Private DICT to know where localSubrs
	// lands relative to it. Build once with a rough subrs offset and pad to
	// stability: since we use a small int, the encoding is 2 bytes fixed.
	// We'll iterate until stable.
	privSize := len(privateDictWithoutSubrs(0))
	subrsOffset := privSize
	for {
		p := privateDictWithoutSubrs(subrsOffset)
		if len(p) == privSize {
			break
		}
		privSize = len(p)
		subrsOffset = privSize
	}
	priv := privateDictWithoutSubrs(subrsOffset)

	// Now lay out the table. We iterate because Top DICT operands reference
	// absolute offsets, and the Top DICT's own length depends on those
	// operands — but with 2-byte fixed-width int encoding we can converge
	// in one pass.
	encTopDict := func(charStringsOffset, privateOffset int) []byte {
		var b bytes.Buffer
		b.Write(encodeInt(charStringsOffset))
		b.Write(encodeOp(17)) // CharStrings
		b.Write(encodeInt(privSize))
		b.Write(encodeInt(privateOffset))
		b.Write(encodeOp(18)) // Private (size, offset)
		return b.Bytes()
	}
	// Force 3-byte encoding of offsets so the Top DICT size is stable.
	// Easier: pick offsets large enough to force 3-byte encoding. We'll
	// just iterate.
	charStringsOffset := 0
	privateOffset := 0
	for iter := 0; iter < 8; iter++ {
		top := encTopDict(charStringsOffset, privateOffset)
		topIdx := encodeIndex([][]byte{top})
		hdrSize := 4
		// We need a String INDEX and Global Subrs INDEX — both empty.
		layout := 0
		nameOff := hdrSize
		topOff := nameOff + len(nameIdx)
		stringsOff := topOff + len(topIdx)
		globalSubrsOff := stringsOff + len(empty)
		charStringsNewOff := globalSubrsOff + len(empty)
		privateNewOff := charStringsNewOff + len(charStrings)
		_ = layout
		if charStringsOffset == charStringsNewOff && privateOffset == privateNewOff {
			// Stable.
			break
		}
		charStringsOffset = charStringsNewOff
		privateOffset = privateNewOff
		if iter == 7 {
			t.Fatal("CFF layout did not converge")
		}
	}

	// Final build.
	topDict := encTopDict(charStringsOffset, privateOffset)
	topIdx := encodeIndex([][]byte{topDict})

	var cff bytes.Buffer
	cff.WriteByte(1) // major
	cff.WriteByte(0) // minor
	cff.WriteByte(4) // hdrSize
	cff.WriteByte(2) // offSize (unused by our parser)
	cff.Write(nameIdx)
	cff.Write(topIdx)
	cff.Write(empty)      // String INDEX
	cff.Write(empty)      // Global Subrs INDEX
	cff.Write(charStrings)
	cff.Write(priv)
	cff.Write(localSubrs)

	f, err := Parse(cff.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := f.PostScriptName, "MyTestFont"; got != want {
		t.Errorf("PostScriptName: got %q, want %q", got, want)
	}
	if got, want := f.NumGlyphs, 1; got != want {
		t.Errorf("NumGlyphs: got %d, want %d", got, want)
	}
	if len(f.CharStrings) != 1 || len(f.CharStrings[0]) != 1 || f.CharStrings[0][0] != 14 {
		t.Errorf("CharStrings: got %v, want [[14]]", f.CharStrings)
	}
	if f.DefaultWidthX != 500 {
		t.Errorf("DefaultWidthX: got %v, want 500", f.DefaultWidthX)
	}
	if f.NominalWidthX != 600 {
		t.Errorf("NominalWidthX: got %v, want 600", f.NominalWidthX)
	}
	if f.IsCIDKeyed {
		t.Error("IsCIDKeyed should be false for a SID font")
	}
}

// TestParseEmptyIndex confirms parseIndex returns a zero-element slice for
// count=0, consuming exactly 2 bytes.
func TestParseEmptyIndex(t *testing.T) {
	data := []byte{0, 0}
	objs, next, err := parseIndex(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Errorf("len(objs): got %d, want 0", len(objs))
	}
	if next != 2 {
		t.Errorf("next: got %d, want 2", next)
	}
}

// TestParseIndexBasic round-trips a small INDEX of two entries.
func TestParseIndexBasic(t *testing.T) {
	data := encodeIndex([][]byte{[]byte("hello"), []byte("world!")})
	objs, next, err := parseIndex(data, 0)
	if err != nil {
		t.Fatalf("parseIndex: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("len(objs): got %d, want 2", len(objs))
	}
	if string(objs[0]) != "hello" || string(objs[1]) != "world!" {
		t.Errorf("objs: got %v, want [\"hello\", \"world!\"]",
			[]string{string(objs[0]), string(objs[1])})
	}
	if next != len(data) {
		t.Errorf("next: got %d, want %d", next, len(data))
	}
}

// TestReadOperandInteger exercises each branch of readOperand.
func TestReadOperandInteger(t *testing.T) {
	for _, tc := range []struct {
		in   []byte
		want float64
		n    int
	}{
		{encodeInt(0), 0, 1},
		{encodeInt(100), 100, 1},
		{encodeInt(-100), -100, 1},
		{encodeInt(500), 500, 2},
		{encodeInt(-500), -500, 2},
		{encodeInt(32000), 32000, 3},
		{encodeInt(-32000), -32000, 3},
		{encodeInt(1 << 20), 1 << 20, 5},
	} {
		v, n, err := readOperand(tc.in)
		if err != nil {
			t.Errorf("readOperand(%v): err = %v", tc.in, err)
			continue
		}
		if v != tc.want || n != tc.n {
			t.Errorf("readOperand(%v): got (%v, %d), want (%v, %d)", tc.in, v, n, tc.want, tc.n)
		}
	}
}
