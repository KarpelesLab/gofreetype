// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

import (
	"fmt"

	"github.com/KarpelesLab/gofreetype/cff"
)

// A Type 1 charstring uses a similar stack-based design to Type 2, but
// with a different opcode table and a handful of semantic differences.
// The big ones:
//
//   * Numeric operands match the first 32..255 opcode encoding used by
//     CFF Type 2 (no 16.16 fixed-point form).
//   * hsbw / sbw declare the glyph width + left side bearing at the
//     start of the charstring (instead of the optional first stack
//     operand convention used by Type 2).
//   * closepath explicitly closes a subpath (Type 2 closes implicitly).
//   * seac composes an accented glyph from two base glyphs.
//
// We translate a Type 1 charstring into the same cff.Segment stream
// produced by the Type 2 interpreter so downstream consumers (truetype,
// the rasterizer) don't care which charstring flavor they came from.
//
// Currently unsupported: seac (accent composition). A glyph whose
// charstring executes seac will raise UnsupportedError; the caller can
// catch that and fall back to drawing the base glyph alone.

// Glyph is the rendered output of a Type 1 charstring.
type Glyph struct {
	Segments    []cff.Segment
	Width       float64
	SideBearing float64
}

// Decode interprets a (decrypted) Type 1 charstring and returns the
// resulting glyph. The subrs slice is the font's /Subrs array, each
// entry being a decrypted charstring body.
func Decode(cs []byte, subrs [][]byte) (*Glyph, error) {
	p := &interp{subrs: subrs}
	if err := p.run(cs); err != nil {
		return nil, err
	}
	return &Glyph{
		Segments:    p.segments,
		Width:       p.width,
		SideBearing: p.sideBearing,
	}, nil
}

type interp struct {
	subrs [][]byte

	stack    [48]float64
	sp       int
	segments []cff.Segment

	x, y        float64
	width       float64
	sideBearing float64

	// closepath: Type 1 outlines typically end subpaths with closepath
	// before starting a new moveto. Our cff.Segment stream starts a new
	// contour on each MoveTo, so closepath is effectively a no-op for
	// the path builder.
	ops int
}

const maxType1Ops = 100000

func (p *interp) push(v float64) error {
	if p.sp >= len(p.stack) {
		return FormatError("charstring stack overflow")
	}
	p.stack[p.sp] = v
	p.sp++
	return nil
}

func (p *interp) run(code []byte) error {
	i := 0
	for i < len(code) {
		p.ops++
		if p.ops > maxType1Ops {
			return FormatError("charstring op budget exceeded")
		}
		b := code[i]
		// Operands.
		switch {
		case b >= 32 && b <= 246:
			if err := p.push(float64(int(b) - 139)); err != nil {
				return err
			}
			i++
			continue
		case b >= 247 && b <= 250:
			if i+1 >= len(code) {
				return FormatError("truncated operand 247..250")
			}
			w := (int(b)-247)*256 + int(code[i+1]) + 108
			if err := p.push(float64(w)); err != nil {
				return err
			}
			i += 2
			continue
		case b >= 251 && b <= 254:
			if i+1 >= len(code) {
				return FormatError("truncated operand 251..254")
			}
			w := -(int(b)-251)*256 - int(code[i+1]) - 108
			if err := p.push(float64(w)); err != nil {
				return err
			}
			i += 2
			continue
		case b == 255:
			if i+4 >= len(code) {
				return FormatError("truncated 32-bit operand")
			}
			v := int32(uint32(code[i+1])<<24 | uint32(code[i+2])<<16 |
				uint32(code[i+3])<<8 | uint32(code[i+4]))
			if err := p.push(float64(v)); err != nil {
				return err
			}
			i += 5
			continue
		}
		// Operator.
		op := uint16(b)
		i++
		if b == 12 {
			if i >= len(code) {
				return FormatError("truncated escape operator")
			}
			op = 0x0c00 | uint16(code[i])
			i++
		}
		done, err := p.apply(op)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return nil
}

func (p *interp) apply(op uint16) (done bool, err error) {
	switch op {
	// Path construction — same operators as Type 2 where the numeric
	// IDs overlap, but some of the Type 2 operators (e.g. hintmask,
	// callgsubr, flex) aren't valid in Type 1.
	case 21: // rmoveto
		if p.sp < 2 {
			return false, FormatError("rmoveto: stack underflow")
		}
		p.moveTo(p.stack[0], p.stack[1])
		p.sp = 0
	case 22: // hmoveto
		if p.sp < 1 {
			return false, FormatError("hmoveto: stack underflow")
		}
		p.moveTo(p.stack[0], 0)
		p.sp = 0
	case 4: // vmoveto
		if p.sp < 1 {
			return false, FormatError("vmoveto: stack underflow")
		}
		p.moveTo(0, p.stack[0])
		p.sp = 0
	case 5: // rlineto
		if p.sp < 2 || p.sp%2 != 0 {
			return false, FormatError("rlineto: bad operand count")
		}
		for j := 0; j+1 < p.sp; j += 2 {
			p.lineTo(p.stack[j], p.stack[j+1])
		}
		p.sp = 0
	case 6: // hlineto
		if p.sp < 1 {
			return false, FormatError("hlineto: stack underflow")
		}
		p.lineTo(p.stack[0], 0)
		p.sp = 0
	case 7: // vlineto
		if p.sp < 1 {
			return false, FormatError("vlineto: stack underflow")
		}
		p.lineTo(0, p.stack[0])
		p.sp = 0
	case 8: // rrcurveto
		if p.sp < 6 || p.sp%6 != 0 {
			return false, FormatError("rrcurveto: bad operand count")
		}
		for j := 0; j+5 < p.sp; j += 6 {
			p.curveTo(p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3],
				p.stack[j+4], p.stack[j+5])
		}
		p.sp = 0
	case 30: // vhcurveto (4 args)
		if p.sp != 4 {
			return false, FormatError("vhcurveto: need 4 operands")
		}
		p.curveTo(0, p.stack[0], p.stack[1], p.stack[2], p.stack[3], 0)
		p.sp = 0
	case 31: // hvcurveto (4 args)
		if p.sp != 4 {
			return false, FormatError("hvcurveto: need 4 operands")
		}
		p.curveTo(p.stack[0], 0, p.stack[1], p.stack[2], 0, p.stack[3])
		p.sp = 0

	// closepath is a no-op for our segment builder — the renderer
	// implicitly closes at the next moveto.
	case 9:
		p.sp = 0

	// Width / side bearing.
	case 13: // hsbw: sbx, wx
		if p.sp < 2 {
			return false, FormatError("hsbw: need 2 operands")
		}
		p.sideBearing = p.stack[0]
		p.width = p.stack[1]
		p.x = p.stack[0]
		p.y = 0
		p.sp = 0
	case 0x0c07: // sbw: sbx sby wx wy
		if p.sp < 4 {
			return false, FormatError("sbw: need 4 operands")
		}
		p.sideBearing = p.stack[0]
		p.width = p.stack[2]
		p.x = p.stack[0]
		p.y = p.stack[1]
		p.sp = 0

	// Hints: we don't apply hinting, just discard.
	case 1, 3: // hstem / vstem
		p.sp = 0
	case 0x0c00: // dotsection
	case 0x0c01: // vstem3
		p.sp = 0
	case 0x0c02: // hstem3
		p.sp = 0

	// Finishing.
	case 14: // endchar
		return true, nil

	// Subroutine calls.
	case 10: // callsubr
		if p.sp < 1 {
			return false, FormatError("callsubr: empty stack")
		}
		idx := int(p.stack[p.sp-1])
		p.sp--
		if idx < 0 || idx >= len(p.subrs) {
			return false, FormatError(fmt.Sprintf("subr index %d out of range", idx))
		}
		if err := p.run(p.subrs[idx]); err != nil {
			return false, err
		}
	case 11: // return
		return true, nil

	// Accent composition — not yet implemented.
	case 0x0c06: // seac
		return false, UnsupportedError("seac (accented glyph composition)")

	// Numeric/other escape ops — div, pop, callothersubr, setcurrentpoint.
	case 0x0c0c: // div
		if p.sp < 2 {
			return false, FormatError("div: need 2 operands")
		}
		num := p.stack[p.sp-2]
		den := p.stack[p.sp-1]
		if den == 0 {
			return false, FormatError("div by zero")
		}
		p.sp -= 2
		if err := p.push(num / den); err != nil {
			return false, err
		}
	case 0x0c10: // callothersubr
		// The common use is Flex: othersubr 0, 1, 2, 3 implement the
		// Type 1 Flex mechanism. Supporting Flex properly requires state
		// across multiple calls; for now we drop the arguments and hope
		// the font has a fallback path.
		if p.sp >= 2 {
			p.sp -= 2 // drop othersubr# and n
		}
	case 0x0c11: // pop
		// Used after callothersubr to move results back onto the stack.
		// With our stubbed callothersubr there's nothing meaningful to
		// pop, so leave it alone.
	case 0x0c21: // setcurrentpoint
		if p.sp >= 2 {
			p.x = p.stack[p.sp-2]
			p.y = p.stack[p.sp-1]
			p.sp = 0
		}
	default:
		return false, FormatError(fmt.Sprintf("unknown Type 1 operator 0x%04x", op))
	}
	return false, nil
}

func (p *interp) moveTo(dx, dy float64) {
	p.x += dx
	p.y += dy
	p.segments = append(p.segments, cff.Segment{Op: cff.SegMoveTo, X: p.x, Y: p.y})
}

func (p *interp) lineTo(dx, dy float64) {
	p.x += dx
	p.y += dy
	p.segments = append(p.segments, cff.Segment{Op: cff.SegLineTo, X: p.x, Y: p.y})
}

func (p *interp) curveTo(dxa, dya, dxb, dyb, dxc, dyc float64) {
	c1x := p.x + dxa
	c1y := p.y + dya
	c2x := c1x + dxb
	c2y := c1y + dyb
	p.x = c2x + dxc
	p.y = c2y + dyc
	p.segments = append(p.segments, cff.Segment{
		Op:  cff.SegCubicTo,
		CX1: c1x, CY1: c1y,
		CX2: c2x, CY2: c2y,
		X: p.x, Y: p.y,
	})
}
