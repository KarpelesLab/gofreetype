// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/KarpelesLab/gofreetype/cff"
)

// buildMiniPFB builds a minimal PFB file with one glyph named "A" whose
// charstring draws a tiny rectangle. The header carries enough of the
// real dictionary keywords (FontName, FontMatrix, FontBBox) for the
// parser to harvest them.
func buildMiniPFB() []byte {
	header := []byte(
		"%!PS-AdobeFont-1.0: Test\n" +
			"/FontName /Test def\n" +
			"/FontMatrix [0.001 0 0 0.001 0 0] def\n" +
			"/FontBBox [0 -100 500 800] def\n" +
			"currentfile eexec\n",
	)

	// Build a plain-text "decrypted eexec body" string that mimics the
	// real Type 1 private-dict layout: /Private dict with /lenIV and
	// /CharStrings inside.
	//
	// We bake the CharStrings directly after a "/CharStrings 1 dict dup
	// begin" marker. Each charstring is emitted as
	//     "/A <length> -| <body> |-"
	// where <body> is the encrypted charstring bytes.
	csAPlain := []byte{
		// hsbw 0 500
		0x8b,     // push 0 (b=139)
		247, 140, // push 500 (500-108=392 -> 247 + (392>>8), 392&0xff). 392 >> 8 = 1, 247+1=248; &0xff=136.
		13, // hsbw
		// rmoveto 10 10
		encNum1(10), encNum1(10),
		21, // rmoveto
		// rlineto 20 0
		encNum1(20), encNum1(0),
		5,
		// rlineto 0 30
		encNum1(0), encNum1(30),
		5,
		// rlineto -20 0
		encNumNeg1(-20), encNum1(0),
		5,
		9,  // closepath
		14, // endchar
	}
	// Fix the 500-encoding — easier to use the helper.
	csAPlain = []byte{}
	for _, n := range []int{0, 500} {
		csAPlain = append(csAPlain, encNumBytes(n)...)
	}
	csAPlain = append(csAPlain, 13) // hsbw
	csAPlain = append(csAPlain, encNumBytes(10)...)
	csAPlain = append(csAPlain, encNumBytes(10)...)
	csAPlain = append(csAPlain, 21) // rmoveto
	csAPlain = append(csAPlain, encNumBytes(20)...)
	csAPlain = append(csAPlain, encNumBytes(0)...)
	csAPlain = append(csAPlain, 5)
	csAPlain = append(csAPlain, encNumBytes(0)...)
	csAPlain = append(csAPlain, encNumBytes(30)...)
	csAPlain = append(csAPlain, 5)
	csAPlain = append(csAPlain, encNumBytes(-20)...)
	csAPlain = append(csAPlain, encNumBytes(0)...)
	csAPlain = append(csAPlain, 5)
	csAPlain = append(csAPlain, 9, 14)

	// Encrypt with 4 lead bytes.
	csA := encryptCharstring(csAPlain, 4)

	// Assemble an eexec-encryptable body.
	var eexecPlain bytes.Buffer
	eexecPlain.WriteString("dup /Private 5 dict dup begin\n")
	eexecPlain.WriteString("/lenIV 4 def\n")
	eexecPlain.WriteString("/Subrs 0 array def\n")
	// CharStrings dict, one glyph "A".
	fmt.Fprintf(&eexecPlain, "/CharStrings 1 dict dup begin\n/A %d -| ", len(csA))
	eexecPlain.Write(csA)
	eexecPlain.WriteString(" |-\nend\nend\n")

	encryptedBody := encryptEexec(eexecPlain.Bytes())

	trailer := []byte("mark currentfile closefile\ncleartomark\n")

	// Build PFB wrapper: [ASCII header][BINARY body][ASCII trailer][EOF].
	var pfb []byte
	appendSeg := func(typ byte, body []byte) {
		pfb = append(pfb, 0x80, typ)
		n := uint32(len(body))
		pfb = append(pfb, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
		pfb = append(pfb, body...)
	}
	appendSeg(pfbSegmentASCII, header)
	appendSeg(pfbSegmentBinary, encryptedBody)
	appendSeg(pfbSegmentASCII, trailer)
	pfb = append(pfb, 0x80, pfbSegmentEOF)
	return pfb
}

func encNumBytes(v int) []byte { return encNum(v) }

// Silence unused-var warnings in the scaffolding above.
func encNum1(v int) byte { return encNum(v)[0] }

func encNumNeg1(v int) byte { return encNum(v)[0] }

func TestParseMiniPFB(t *testing.T) {
	f, err := Parse(buildMiniPFB())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.FontName != "Test" {
		t.Errorf("FontName: got %q, want Test", f.FontName)
	}
	if f.FontMatrix[0] != 0.001 {
		t.Errorf("FontMatrix[0]: got %v, want 0.001", f.FontMatrix[0])
	}
	if f.FontBBox != ([4]float64{0, -100, 500, 800}) {
		t.Errorf("FontBBox: got %v, want [0 -100 500 800]", f.FontBBox)
	}
	if _, ok := f.CharStrings["A"]; !ok {
		t.Fatalf("CharStrings missing 'A'; got keys %v", keys(f.CharStrings))
	}
	g, err := f.LoadGlyph("A")
	if err != nil {
		t.Fatalf("LoadGlyph: %v", err)
	}
	if g.Width != 500 {
		t.Errorf("Width: got %v, want 500", g.Width)
	}
	wantOps := []cff.SegmentOp{cff.SegMoveTo, cff.SegLineTo, cff.SegLineTo, cff.SegLineTo}
	if len(g.Segments) != len(wantOps) {
		t.Fatalf("Segments: got %d, want %d", len(g.Segments), len(wantOps))
	}
	for i, op := range wantOps {
		if g.Segments[i].Op != op {
			t.Errorf("seg[%d] op: got %d, want %d", i, g.Segments[i].Op, op)
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
