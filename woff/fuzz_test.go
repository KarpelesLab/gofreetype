// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package woff

import "testing"

// FuzzDecode verifies that malformed WOFF input is rejected cleanly —
// never with a panic, OOM, or infinite loop.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("wOFF"))
	f.Add([]byte("wOF2"))
	f.Add([]byte("wOFF\x00\x01\x00\x00\x00\x00\x00\x2c\x00\x00\x00\x00")) // truncated header
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(data)
		_ = IsWOFF(data)
	})
}
