// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import (
	"fmt"
	"strconv"
)

// A CFF DICT is a sequence of (operand*, operator) pairs. Operands are either
// integers or "real" numbers, encoded using a byte-prefix scheme described in
// Adobe Technical Note #5176 section 4.

// topDict holds the fields we care about from a Top DICT.
type topDict struct {
	charStringType    int
	hasFontMatrix     bool
	fontMatrix        [6]float64
	charStringsOffset int
	privateSize       int
	privateOffset     int
	fdArrayOffset     int
	fdSelectOffset    int
	charsetOffset     int
	isCID             bool
}

// privateDict holds the fields we care about from a Private DICT.
type privateDict struct {
	defaultWidthX float64
	nominalWidthX float64
	subrsOffset   int
}

func parseTopDict(data []byte) (*topDict, error) {
	td := &topDict{charStringType: 2}
	var operands []float64
	i := 0
	for i < len(data) {
		b := data[i]
		if b <= 21 {
			op := uint16(b)
			i++
			if b == 12 {
				if i >= len(data) {
					return nil, FormatError("truncated DICT operator")
				}
				op = 12<<8 | uint16(data[i])
				i++
			}
			if err := applyTopOp(td, op, operands); err != nil {
				return nil, err
			}
			operands = operands[:0]
			continue
		}
		v, n, err := readOperand(data[i:])
		if err != nil {
			return nil, err
		}
		operands = append(operands, v)
		i += n
	}
	return td, nil
}

func parsePrivateDict(data []byte) (*privateDict, error) {
	pd := &privateDict{}
	var operands []float64
	i := 0
	for i < len(data) {
		b := data[i]
		if b <= 21 {
			op := uint16(b)
			i++
			if b == 12 {
				if i >= len(data) {
					return nil, FormatError("truncated DICT operator")
				}
				op = 12<<8 | uint16(data[i])
				i++
			}
			applyPrivateOp(pd, op, operands)
			operands = operands[:0]
			continue
		}
		v, n, err := readOperand(data[i:])
		if err != nil {
			return nil, err
		}
		operands = append(operands, v)
		i += n
	}
	return pd, nil
}

func applyTopOp(td *topDict, op uint16, opr []float64) error {
	switch op {
	case 0x0c06: // CharStringType
		if len(opr) == 1 {
			td.charStringType = int(opr[0])
		}
	case 0x0c07: // FontMatrix
		if len(opr) == 6 {
			td.hasFontMatrix = true
			copy(td.fontMatrix[:], opr)
		}
	case 15: // charset
		if len(opr) == 1 {
			td.charsetOffset = int(opr[0])
		}
	case 16: // Encoding — offset (unused by us currently)
	case 17: // CharStrings
		if len(opr) == 1 {
			td.charStringsOffset = int(opr[0])
		}
	case 18: // Private (size, offset)
		if len(opr) == 2 {
			td.privateSize = int(opr[0])
			td.privateOffset = int(opr[1])
		}
	case 0x0c1e: // ROS — CID marker
		td.isCID = true
	case 0x0c22: // FDArray
		if len(opr) == 1 {
			td.fdArrayOffset = int(opr[0])
		}
	case 0x0c23: // FDSelect
		if len(opr) == 1 {
			td.fdSelectOffset = int(opr[0])
		}
	}
	return nil
}

func applyPrivateOp(pd *privateDict, op uint16, opr []float64) {
	switch op {
	case 20: // defaultWidthX
		if len(opr) == 1 {
			pd.defaultWidthX = opr[0]
		}
	case 21: // nominalWidthX
		if len(opr) == 1 {
			pd.nominalWidthX = opr[0]
		}
	case 19: // Subrs (offset relative to Private DICT)
		if len(opr) == 1 {
			pd.subrsOffset = int(opr[0])
		}
	}
}

// readOperand decodes a single operand (integer or real) from the head of b.
// It returns the numeric value, how many bytes were consumed, and any error.
// See Adobe Technical Note #5176 Table 3.
func readOperand(b []byte) (float64, int, error) {
	if len(b) == 0 {
		return 0, 0, FormatError("truncated DICT operand")
	}
	b0 := b[0]
	switch {
	case b0 >= 32 && b0 <= 246:
		return float64(int(b0) - 139), 1, nil
	case b0 >= 247 && b0 <= 250:
		if len(b) < 2 {
			return 0, 0, FormatError("truncated DICT operand (247..250)")
		}
		return float64((int(b0)-247)*256 + int(b[1]) + 108), 2, nil
	case b0 >= 251 && b0 <= 254:
		if len(b) < 2 {
			return 0, 0, FormatError("truncated DICT operand (251..254)")
		}
		return float64(-(int(b0)-251)*256 - int(b[1]) - 108), 2, nil
	case b0 == 28:
		if len(b) < 3 {
			return 0, 0, FormatError("truncated DICT short-int operand")
		}
		return float64(int16(uint16(b[1])<<8 | uint16(b[2]))), 3, nil
	case b0 == 29:
		if len(b) < 5 {
			return 0, 0, FormatError("truncated DICT long-int operand")
		}
		return float64(int32(uint32(b[1])<<24 | uint32(b[2])<<16 | uint32(b[3])<<8 | uint32(b[4]))), 5, nil
	case b0 == 30:
		return readRealOperand(b)
	}
	return 0, 0, FormatError(fmt.Sprintf("bad DICT operand byte 0x%02x", b0))
}

// readRealOperand decodes a nibble-packed "real" operand starting at b[0]==30.
// Nibbles:
//
//	0..9   decimal digit
//	0xa    '.'
//	0xb    'E'
//	0xc    'E-'
//	0xd    reserved
//	0xe    '-'
//	0xf    end of number
func readRealOperand(b []byte) (float64, int, error) {
	if len(b) < 2 {
		return 0, 0, FormatError("truncated real DICT operand")
	}
	var buf [64]byte
	n := 0
	i := 1
	for i < len(b) {
		hi := b[i] >> 4
		lo := b[i] & 0x0f
		for _, nibble := range [2]byte{hi, lo} {
			switch {
			case nibble <= 9:
				if n < len(buf) {
					buf[n] = '0' + nibble
					n++
				}
			case nibble == 0x0a:
				if n < len(buf) {
					buf[n] = '.'
					n++
				}
			case nibble == 0x0b:
				if n < len(buf) {
					buf[n] = 'e'
					n++
				}
			case nibble == 0x0c:
				if n+1 < len(buf) {
					buf[n] = 'e'
					buf[n+1] = '-'
					n += 2
				}
			case nibble == 0x0e:
				if n < len(buf) {
					buf[n] = '-'
					n++
				}
			case nibble == 0x0f:
				v, err := parseFloatAscii(string(buf[:n]))
				if err != nil {
					return 0, 0, FormatError(fmt.Sprintf("bad real operand %q", string(buf[:n])))
				}
				return v, i + 1, nil
			}
		}
		i++
	}
	return 0, 0, FormatError("unterminated real DICT operand")
}

func parseFloatAscii(s string) (float64, error) {
	if len(s) == 0 {
		return 0, FormatError("empty real operand")
	}
	return strconv.ParseFloat(s, 64)
}
