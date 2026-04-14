// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

// Type 1 uses a rolling 16-bit XOR cipher keyed on the previous
// ciphertext byte. The constants come straight from the Type 1 spec:
//
//   R_eexec = 55665
//   R_charstring = 4330
//   c1 = 52845
//   c2 = 22719
//
// Each source byte c produces plaintext = c XOR (R >> 8), then
// R = (c + R) * c1 + c2 mod 65536 advances the state.
//
// eexec adds a wrinkle: the first four plaintext bytes are "random"
// lead bytes that the decoder must discard. The cipher also supports
// ASCII hex input (two hex digits per cipher byte) but modern PFB
// files always embed the binary form.

const (
	eexecKey       uint16 = 55665
	charstringKey  uint16 = 4330
	cipherC1       uint16 = 52845
	cipherC2       uint16 = 22719
	eexecLeadBytes        = 4
	charstringLeadBytes   = 4 // configurable via lenIV, default 4
)

// decryptEexec undoes the eexec encryption over an entire binary
// segment. The first 4 decrypted bytes are the random-lead marker and
// are trimmed from the output.
func decryptEexec(src []byte) []byte {
	out := decrypt(src, eexecKey)
	if len(out) < eexecLeadBytes {
		return nil
	}
	return out[eexecLeadBytes:]
}

// decryptCharstring decrypts a single charstring with the specified
// number of leading random bytes (from the font's lenIV entry).
func decryptCharstring(src []byte, leadBytes int) []byte {
	out := decrypt(src, charstringKey)
	if len(out) < leadBytes {
		return nil
	}
	return out[leadBytes:]
}

// decrypt runs the core Type 1 cipher starting from the given R value.
func decrypt(src []byte, r uint16) []byte {
	out := make([]byte, len(src))
	for i, c := range src {
		out[i] = byte(uint16(c) ^ (r >> 8))
		r = (uint16(c)+r)*cipherC1 + cipherC2
	}
	return out
}

// encryptEexec is the inverse of decryptEexec, used only by tests to
// round-trip sample byte sequences.
func encryptEexec(src []byte) []byte {
	return encrypt(append([]byte{0, 0, 0, 0}, src...), eexecKey)
}

// encryptCharstring is the inverse of decryptCharstring.
func encryptCharstring(src []byte, leadBytes int) []byte {
	lead := make([]byte, leadBytes)
	return encrypt(append(lead, src...), charstringKey)
}

func encrypt(src []byte, r uint16) []byte {
	out := make([]byte, len(src))
	for i, plain := range src {
		c := byte(uint16(plain) ^ (r >> 8))
		out[i] = c
		r = (uint16(c)+r)*cipherC1 + cipherC2
	}
	return out
}
