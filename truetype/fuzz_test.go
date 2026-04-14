// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"io/ioutil"
	"testing"

	"golang.org/x/image/font"
)

// FuzzParse feeds arbitrary bytes to Parse. A parse failure is always
// acceptable; a panic is not. This catches out-of-bounds reads, unchecked
// length assumptions, and malformed-table overflow bugs in the SFNT,
// cmap, glyf, and (optionally) CFF / OT Layout / variable-font parsers.
//
// Run with:
//
//	go test -fuzz=FuzzParse ./truetype/
func FuzzParse(f *testing.F) {
	// Seed with the real luxisr.ttf plus a handful of edge-case inputs.
	if data, err := ioutil.ReadFile("../testdata/luxisr.ttf"); err == nil {
		f.Add(data)
	}
	f.Add([]byte{})
	f.Add([]byte{0, 1, 0, 0}) // truncated SFNT magic
	f.Add([]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x02\x00\x00\x00\x0c"))
	f.Add([]byte("wOFF"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse must never panic on arbitrary input.
		ft, err := Parse(data)
		if err != nil || ft == nil {
			return
		}
		// If Parse succeeds, try loading glyph 0 at a few scales. Loading
		// should fail or succeed cleanly, not panic.
		g := &GlyphBuf{}
		_ = g.Load(ft, 1<<6, 0, font.HintingNone)
		_ = g.Load(ft, 12<<6, 0, font.HintingNone)
		// Also exercise the face path, which has its own cache logic.
		face := NewFace(ft, &Options{Size: 10, DPI: 72})
		face.Close()
	})
}

// FuzzParseIndex hardens the TTC dispatch path.
func FuzzParseIndex(f *testing.F) {
	if data, err := ioutil.ReadFile("../testdata/luxisr.ttf"); err == nil {
		f.Add(data, 0)
		f.Add(data, 1)
		f.Add(data, -1)
	}
	f.Fuzz(func(t *testing.T, data []byte, idx int) {
		_, _ = ParseIndex(data, idx)
		_, _ = NumFonts(data)
	})
}
