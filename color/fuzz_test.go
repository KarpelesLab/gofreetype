// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import "testing"

func FuzzParseCPAL(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 2, 0, 1, 0, 2, 0, 0, 0, 12, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseCPAL(data) })
}

func FuzzParseCOLR(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 14, 0, 0, 0, 14, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseCOLR(data) })
}

func FuzzParseSVG(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 0, 10, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseSVG(data) })
}

func FuzzParseSbix(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 8})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = ParseSbix(data, 16) })
}
