// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte{})
	f.Add(buildLayoutTable())
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = Parse(data) })
}
