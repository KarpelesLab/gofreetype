// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "testing"

// FuzzParse exercises the CFF v1 container parser with arbitrary input.
// A malformed CFF must produce an error, never a panic. Seeded with a
// single known-good minimal CFF and a few short invalid prefixes.
func FuzzParse(f *testing.F) {
	// Minimal-CFF helper (built by test TestParseMinimalCFF already) is
	// not exported, so we add a few raw seeds and let the fuzzer explore.
	f.Add([]byte{})
	f.Add([]byte{1, 0, 4, 2})       // just the header, no INDEXes
	f.Add([]byte{1, 0, 4, 2, 0, 0}) // header + empty Name INDEX
	f.Add([]byte{2, 0, 5, 0})       // wrong major version
	f.Fuzz(func(t *testing.T, data []byte) {
		font, err := Parse(data)
		if err != nil || font == nil {
			return
		}
		for i := 0; i < font.NumGlyphs && i < 32; i++ {
			_, _ = font.LoadGlyph(i)
		}
	})
}
