// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import (
	"fmt"
	"math"
)

// A SegmentOp identifies which kind of path segment a Segment represents.
type SegmentOp uint8

const (
	// SegMoveTo starts a new contour at (X, Y).
	SegMoveTo SegmentOp = iota
	// SegLineTo draws a straight line from the current point to (X, Y).
	SegLineTo
	// SegCubicTo draws a cubic Bézier from the current point through
	// (CX1, CY1) and (CX2, CY2) to (X, Y).
	SegCubicTo
)

// A Segment is one path operation emitted by the Type 2 interpreter, in the
// font's integer unit coordinate system (before applying FontMatrix).
type Segment struct {
	Op       SegmentOp
	X, Y     float64
	CX1, CY1 float64
	CX2, CY2 float64
}

// Glyph holds the rendered output of a Type 2 charstring.
type Glyph struct {
	// Segments describe the glyph outline. A SegMoveTo starts a new
	// contour; the final closing line is implicit (done by the renderer).
	Segments []Segment

	// Width is the glyph advance width in font units, decoded from the
	// charstring (falling back to DefaultWidthX when the charstring carries
	// no explicit width).
	Width float64

	// HasWidth reports whether the charstring explicitly provided a width
	// (as the optional first operand).
	HasWidth bool
}

// LoadGlyph interprets a Type 2 charstring for glyph index gid and returns
// the resulting Glyph.
func (f *Font) LoadGlyph(gid int) (*Glyph, error) {
	if gid < 0 || gid >= len(f.CharStrings) {
		return nil, fmt.Errorf("cff: glyph index %d out of range [0, %d)", gid, len(f.CharStrings))
	}
	locals := f.LocalSubrs
	nominalWidth := f.NominalWidthX
	defaultWidth := f.DefaultWidthX
	if f.IsCIDKeyed {
		if gid >= len(f.FDSelect) {
			return nil, fmt.Errorf("cff: FDSelect has no entry for glyph %d", gid)
		}
		fd := int(f.FDSelect[gid])
		if fd >= len(f.FDSubrs) {
			return nil, fmt.Errorf("cff: FDSelect references FD %d but only %d FDs", fd, len(f.FDSubrs))
		}
		locals = f.FDSubrs[fd]
		nominalWidth = f.FDNominalWidthX[fd]
		defaultWidth = f.FDDefaultWidthX[fd]
	}

	psInterp := &interp{
		globals:      f.GlobalSubrs,
		locals:       locals,
		nominalWidth: nominalWidth,
		defaultWidth: defaultWidth,
	}
	if err := psInterp.run(f.CharStrings[gid]); err != nil {
		return nil, fmt.Errorf("cff: glyph %d: %w", gid, err)
	}
	return &Glyph{
		Segments: psInterp.segments,
		Width:    psInterp.width,
		HasWidth: psInterp.hasWidth,
	}, nil
}

// interp is the Type 2 charstring stack machine.
type interp struct {
	globals, locals [][]byte

	stack       [96]float64
	sp          int
	segments    []Segment
	x, y        float64
	contourOpen bool

	// callStack tracks nested callsubr/callgsubr recursion.
	callStack [10]subrFrame
	callDepth int

	// Hint counters (we don't implement hinting; we just need to skip
	// hintmask / cntrmask operand bytes whose count depends on how many
	// stems are declared).
	nStemHints int

	nominalWidth float64
	defaultWidth float64
	width        float64
	hasWidth     bool
	widthPopped  bool // we've already decided whether sp[0] was a width

	// hintSink, if non-nil, receives stem declarations as hstem/vstem
	// (and hm variants) execute. Used by LoadGlyphHinted to surface
	// stems alongside the decoded segment stream.
	hintSink *stemSink

	// ops is the number of operators executed; used to bound runaway charstrings.
	ops int
}

type subrFrame struct {
	code []byte
	pc   int
}

const (
	maxCharstringOps = 100000
)

func (p *interp) run(code []byte) error {
	return p.exec(code)
}

func (p *interp) exec(code []byte) error {
	i := 0
	for i < len(code) {
		p.ops++
		if p.ops > maxCharstringOps {
			return fmt.Errorf("charstring operator budget exceeded")
		}
		b := code[i]
		switch {
		case b >= 32:
			// Integer / fixed operand.
			v, n, err := readCSOperand(code[i:])
			if err != nil {
				return err
			}
			if err := p.push(v); err != nil {
				return err
			}
			i += n
			continue
		case b == 28:
			// Short integer (3 bytes).
			if i+2 >= len(code) {
				return fmt.Errorf("truncated charstring short int")
			}
			v := float64(int16(uint16(code[i+1])<<8 | uint16(code[i+2])))
			if err := p.push(v); err != nil {
				return err
			}
			i += 3
			continue
		}
		// Operator.
		op := uint16(b)
		i++
		if b == 12 {
			if i >= len(code) {
				return fmt.Errorf("truncated escape operator")
			}
			op = 0x0c00 | uint16(code[i])
			i++
		}
		done, consumed, err := p.apply(op, code[i:])
		if err != nil {
			return err
		}
		i += consumed
		if done {
			return nil
		}
	}
	return nil
}

func (p *interp) push(v float64) error {
	if p.sp >= len(p.stack) {
		return fmt.Errorf("charstring stack overflow")
	}
	p.stack[p.sp] = v
	p.sp++
	return nil
}

// popWidth consumes the optional width operand from the bottom of the stack
// at the moment a width-carrying operator begins. It must be called exactly
// once per charstring, at the first such operator.
func (p *interp) popWidth(requiredArgCount int) {
	if p.widthPopped {
		return
	}
	p.widthPopped = true
	if p.sp > requiredArgCount {
		// Odd count above the required — first arg is a width delta.
		extra := p.sp - requiredArgCount
		if extra >= 1 {
			p.width = p.nominalWidth + p.stack[0]
			p.hasWidth = true
			copy(p.stack[:p.sp-1], p.stack[1:p.sp])
			p.sp--
		}
	} else {
		p.width = p.defaultWidth
	}
}

// popWidthIfOdd is used by operators whose stack is otherwise a multiple of
// some n — if the actual stack length modulo n is 1, the low element is a
// width.
func (p *interp) popWidthIfOdd() {
	if p.widthPopped {
		return
	}
	p.widthPopped = true
	if p.sp%2 == 1 {
		p.width = p.nominalWidth + p.stack[0]
		p.hasWidth = true
		copy(p.stack[:p.sp-1], p.stack[1:p.sp])
		p.sp--
	} else {
		p.width = p.defaultWidth
	}
}

func (p *interp) apply(op uint16, rest []byte) (done bool, consumed int, err error) {
	switch op {
	// Path construction.
	case 21: // rmoveto
		p.popWidth(2)
		if p.sp < 2 {
			return false, 0, fmt.Errorf("rmoveto needs 2 operands")
		}
		p.closeContour()
		p.moveTo(p.stack[0], p.stack[1])
		p.sp = 0
	case 22: // hmoveto
		p.popWidth(1)
		if p.sp < 1 {
			return false, 0, fmt.Errorf("hmoveto needs 1 operand")
		}
		p.closeContour()
		p.moveTo(p.stack[0], 0)
		p.sp = 0
	case 4: // vmoveto
		p.popWidth(1)
		if p.sp < 1 {
			return false, 0, fmt.Errorf("vmoveto needs 1 operand")
		}
		p.closeContour()
		p.moveTo(0, p.stack[0])
		p.sp = 0
	case 5: // rlineto
		if p.sp < 2 || p.sp%2 != 0 {
			return false, 0, fmt.Errorf("rlineto: bad operand count %d", p.sp)
		}
		for j := 0; j+1 < p.sp; j += 2 {
			p.lineTo(p.stack[j], p.stack[j+1])
		}
		p.sp = 0
	case 6: // hlineto: dx1 {dya dxb}* or {dxa dyb}+ (alternating)
		if p.sp < 1 {
			return false, 0, fmt.Errorf("hlineto needs >= 1 operand")
		}
		horizontal := true
		for j := 0; j < p.sp; j++ {
			if horizontal {
				p.lineTo(p.stack[j], 0)
			} else {
				p.lineTo(0, p.stack[j])
			}
			horizontal = !horizontal
		}
		p.sp = 0
	case 7: // vlineto
		if p.sp < 1 {
			return false, 0, fmt.Errorf("vlineto needs >= 1 operand")
		}
		horizontal := false
		for j := 0; j < p.sp; j++ {
			if horizontal {
				p.lineTo(p.stack[j], 0)
			} else {
				p.lineTo(0, p.stack[j])
			}
			horizontal = !horizontal
		}
		p.sp = 0
	case 8: // rrcurveto: {dxa dya dxb dyb dxc dyc}+
		if p.sp < 6 || p.sp%6 != 0 {
			return false, 0, fmt.Errorf("rrcurveto: bad operand count %d", p.sp)
		}
		for j := 0; j+5 < p.sp; j += 6 {
			p.curveTo(p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3], p.stack[j+4], p.stack[j+5])
		}
		p.sp = 0
	case 24: // rcurveline: {dxa dya dxb dyb dxc dyc}+ dxd dyd
		if p.sp < 8 || (p.sp-2)%6 != 0 {
			return false, 0, fmt.Errorf("rcurveline: bad operand count %d", p.sp)
		}
		n := p.sp - 2
		for j := 0; j+5 < n; j += 6 {
			p.curveTo(p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3], p.stack[j+4], p.stack[j+5])
		}
		p.lineTo(p.stack[n], p.stack[n+1])
		p.sp = 0
	case 25: // rlinecurve: {dxa dya}+ dxb dyb dxc dyc dxd dyd
		if p.sp < 8 || (p.sp-6)%2 != 0 {
			return false, 0, fmt.Errorf("rlinecurve: bad operand count %d", p.sp)
		}
		n := p.sp - 6
		for j := 0; j+1 < n; j += 2 {
			p.lineTo(p.stack[j], p.stack[j+1])
		}
		p.curveTo(p.stack[n], p.stack[n+1], p.stack[n+2], p.stack[n+3], p.stack[n+4], p.stack[n+5])
		p.sp = 0
	case 27: // hhcurveto: [dy1] {dxa dxb dyb dxc}+
		if p.sp < 4 {
			return false, 0, fmt.Errorf("hhcurveto needs >= 4 operands")
		}
		j := 0
		dy1 := 0.0
		if p.sp%4 == 1 {
			dy1 = p.stack[0]
			j = 1
		}
		for j+3 < p.sp {
			dxa, dxb, dyb, dxc := p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3]
			p.curveTo(dxa, dy1, dxb, dyb, dxc, 0)
			dy1 = 0
			j += 4
		}
		p.sp = 0
	case 26: // vvcurveto: [dx1] {dya dxb dyb dyc}+
		if p.sp < 4 {
			return false, 0, fmt.Errorf("vvcurveto needs >= 4 operands")
		}
		j := 0
		dx1 := 0.0
		if p.sp%4 == 1 {
			dx1 = p.stack[0]
			j = 1
		}
		for j+3 < p.sp {
			dya, dxb, dyb, dyc := p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3]
			p.curveTo(dx1, dya, dxb, dyb, 0, dyc)
			dx1 = 0
			j += 4
		}
		p.sp = 0
	case 30: // vhcurveto
		return false, 0, p.runAlternatingCurveto(false)
	case 31: // hvcurveto
		return false, 0, p.runAlternatingCurveto(true)

	// Finishing.
	case 14: // endchar
		p.popWidth(0)
		p.closeContour()
		return true, 0, nil

	// Hints — track stem counts and, optionally, surface stem positions
	// to a hint sink for the caller-driven hinter.
	case 1, 3, 18, 23: // hstem / vstem / hstemhm / vstemhm
		p.popWidthIfOdd()
		if p.sp%2 != 0 {
			return false, 0, fmt.Errorf("stem declaration: odd operand count %d", p.sp)
		}
		p.nStemHints += p.sp / 2
		if p.hintSink != nil {
			horizontal := op == 1 || op == 18
			for j := 0; j+1 < p.sp; j += 2 {
				if horizontal {
					p.hintSink.currentY += p.stack[j]
					p.hintSink.stems = append(p.hintSink.stems, Stem{
						Horizontal: true,
						Edge:       p.hintSink.currentY,
						Width:      p.stack[j+1],
					})
					p.hintSink.currentY += p.stack[j+1]
				} else {
					p.hintSink.currentX += p.stack[j]
					p.hintSink.stems = append(p.hintSink.stems, Stem{
						Horizontal: false,
						Edge:       p.hintSink.currentX,
						Width:      p.stack[j+1],
					})
					p.hintSink.currentX += p.stack[j+1]
				}
			}
		}
		p.sp = 0
	case 19, 20: // hintmask / cntrmask
		// Implicit stem declaration for any remaining operands.
		p.popWidthIfOdd()
		if p.sp%2 != 0 {
			return false, 0, fmt.Errorf("hintmask/cntrmask: odd operand count %d", p.sp)
		}
		p.nStemHints += p.sp / 2
		p.sp = 0
		maskLen := (p.nStemHints + 7) / 8
		if maskLen > len(rest) {
			return false, 0, fmt.Errorf("hintmask/cntrmask: need %d mask bytes, have %d", maskLen, len(rest))
		}
		return false, maskLen, nil

	// Subroutines.
	case 10: // callsubr
		return false, 0, p.callSubr(p.locals, false)
	case 29: // callgsubr
		return false, 0, p.callSubr(p.globals, true)
	case 11: // return
		return false, 0, fmt.Errorf("stray return outside subr")

	// Flex — approximate as straight cubics.
	case 0x0c22: // hflex
		return false, 0, p.doHflex()
	case 0x0c23: // flex
		return false, 0, p.doFlex()
	case 0x0c24: // hflex1
		return false, 0, p.doHflex1()
	case 0x0c25: // flex1
		return false, 0, p.doFlex1()

	default:
		return false, 0, fmt.Errorf("unknown charstring operator 0x%04x", op)
	}
	return false, 0, nil
}

// runAlternatingCurveto handles hvcurveto (startHorizontal=true) and
// vhcurveto (startHorizontal=false). Layout:
//
//	{dx1 dxb dyb dy1}+ [dyf?]         — vhcurveto start vertical
//	{dy1 dxb dyb dx2} {dx3 dxd dyd dy3}+ [dxf?]  — full alternation
func (p *interp) runAlternatingCurveto(startHorizontal bool) error {
	if p.sp < 4 {
		return fmt.Errorf("hvcurveto/vhcurveto needs >= 4 operands")
	}
	j := 0
	horizontal := startHorizontal
	for j < p.sp {
		rem := p.sp - j
		if rem < 4 {
			return fmt.Errorf("hvcurveto/vhcurveto trailing %d operands", rem)
		}
		// Each chunk is 4 or 5 operands; the optional 5th is the trailing
		// coordinate that closes the pattern on the opposite axis.
		trailing5 := rem == 5 || rem == 4
		_ = trailing5
		var tail float64
		if rem == 5 {
			tail = p.stack[j+4]
			rem = 4
		}
		a, b, c, d := p.stack[j], p.stack[j+1], p.stack[j+2], p.stack[j+3]
		if horizontal {
			// First control point lies on the horizontal axis.
			if p.sp-j == 4 {
				p.curveTo(a, 0, b, c, tail, d)
			} else {
				p.curveTo(a, 0, b, c, 0, d)
			}
		} else {
			if p.sp-j == 4 {
				p.curveTo(0, a, b, c, d, tail)
			} else {
				p.curveTo(0, a, b, c, d, 0)
			}
		}
		horizontal = !horizontal
		j += 4
		if p.sp-j == 1 {
			// Consumed the tail too.
			j++
		}
	}
	p.sp = 0
	return nil
}

func (p *interp) moveTo(dx, dy float64) {
	p.x += dx
	p.y += dy
	p.segments = append(p.segments, Segment{Op: SegMoveTo, X: p.x, Y: p.y})
	p.contourOpen = true
}

func (p *interp) lineTo(dx, dy float64) {
	p.x += dx
	p.y += dy
	p.segments = append(p.segments, Segment{Op: SegLineTo, X: p.x, Y: p.y})
}

func (p *interp) curveTo(dxa, dya, dxb, dyb, dxc, dyc float64) {
	c1x := p.x + dxa
	c1y := p.y + dya
	c2x := c1x + dxb
	c2y := c1y + dyb
	p.x = c2x + dxc
	p.y = c2y + dyc
	p.segments = append(p.segments, Segment{
		Op:  SegCubicTo,
		CX1: c1x, CY1: c1y,
		CX2: c2x, CY2: c2y,
		X: p.x, Y: p.y,
	})
}

func (p *interp) closeContour() {
	p.contourOpen = false
}

func (p *interp) callSubr(subrs [][]byte, global bool) error {
	if p.sp < 1 {
		return fmt.Errorf("callsubr with empty stack")
	}
	if p.callDepth >= len(p.callStack) {
		return fmt.Errorf("subr recursion too deep")
	}
	idx := int(p.stack[p.sp-1])
	p.sp--
	idx += subrBias(len(subrs))
	if idx < 0 || idx >= len(subrs) {
		return fmt.Errorf("subr index %d out of range (%d subrs, global=%v)", idx, len(subrs), global)
	}
	return p.execSubr(subrs[idx])
}

// subrBias returns the Type 2 bias that must be added to the indexed-from-0
// subr number from the charstring to get the actual index.
func subrBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	default:
		return 32768
	}
}

func (p *interp) execSubr(code []byte) error {
	p.callDepth++
	defer func() { p.callDepth-- }()

	i := 0
	for i < len(code) {
		p.ops++
		if p.ops > maxCharstringOps {
			return fmt.Errorf("charstring operator budget exceeded")
		}
		b := code[i]
		switch {
		case b >= 32:
			v, n, err := readCSOperand(code[i:])
			if err != nil {
				return err
			}
			if err := p.push(v); err != nil {
				return err
			}
			i += n
			continue
		case b == 28:
			if i+2 >= len(code) {
				return fmt.Errorf("truncated short int in subr")
			}
			v := float64(int16(uint16(code[i+1])<<8 | uint16(code[i+2])))
			if err := p.push(v); err != nil {
				return err
			}
			i += 3
			continue
		}
		op := uint16(b)
		i++
		if b == 12 {
			if i >= len(code) {
				return fmt.Errorf("truncated escape operator in subr")
			}
			op = 0x0c00 | uint16(code[i])
			i++
		}
		if op == 11 { // return
			return nil
		}
		if op == 14 { // endchar terminates the whole charstring even inside subrs
			_, _, err := p.apply(op, code[i:])
			return err
		}
		_, consumed, err := p.apply(op, code[i:])
		if err != nil {
			return err
		}
		i += consumed
	}
	return nil
}

// readCSOperand decodes a Type 2 charstring numeric operand. It handles
// 32..246, 247..254 (two-byte), and 255 (32-bit fixed 16.16). The 28 short-int
// form is handled by the caller because it has a different length.
func readCSOperand(b []byte) (float64, int, error) {
	if len(b) == 0 {
		return 0, 0, fmt.Errorf("truncated operand")
	}
	b0 := b[0]
	switch {
	case b0 >= 32 && b0 <= 246:
		return float64(int(b0) - 139), 1, nil
	case b0 >= 247 && b0 <= 250:
		if len(b) < 2 {
			return 0, 0, fmt.Errorf("truncated operand (247..250)")
		}
		return float64((int(b0)-247)*256 + int(b[1]) + 108), 2, nil
	case b0 >= 251 && b0 <= 254:
		if len(b) < 2 {
			return 0, 0, fmt.Errorf("truncated operand (251..254)")
		}
		return float64(-(int(b0)-251)*256 - int(b[1]) - 108), 2, nil
	case b0 == 255:
		if len(b) < 5 {
			return 0, 0, fmt.Errorf("truncated 32-bit fixed operand")
		}
		raw := int32(uint32(b[1])<<24 | uint32(b[2])<<16 | uint32(b[3])<<8 | uint32(b[4]))
		return float64(raw) / 65536.0, 5, nil
	}
	return 0, 0, fmt.Errorf("bad charstring operand byte 0x%02x", b0)
}

// --- Flex helpers (approximations; flex is rarely load-bearing visually) ---

func (p *interp) doFlex() error {
	if p.sp < 13 {
		return fmt.Errorf("flex needs 13 operands")
	}
	// Two cubic Béziers. The 13th operand is a flex depth threshold in
	// hundredths of a pixel — for rasterization at current resolutions we
	// always emit the curves.
	s := p.stack[:12]
	p.curveTo(s[0], s[1], s[2], s[3], s[4], s[5])
	p.curveTo(s[6], s[7], s[8], s[9], s[10], s[11])
	p.sp = 0
	return nil
}

func (p *interp) doHflex() error {
	if p.sp < 7 {
		return fmt.Errorf("hflex needs 7 operands")
	}
	s := p.stack[:7]
	// First curve: horizontal start + Y delta midway; second curve returns to
	// the original Y.
	p.curveTo(s[0], 0, s[1], s[2], s[3], 0)
	p.curveTo(s[4], 0, s[5], -s[2], s[6], 0)
	p.sp = 0
	return nil
}

func (p *interp) doHflex1() error {
	if p.sp < 9 {
		return fmt.Errorf("hflex1 needs 9 operands")
	}
	s := p.stack[:9]
	// Final Y returns to start; compute the closing Y delta.
	yTotal := s[1] + s[3] + s[5] + s[7]
	p.curveTo(s[0], s[1], s[2], s[3], s[4], 0)
	p.curveTo(s[5], 0, s[6], s[7], s[8], -yTotal+s[1]+s[3])
	p.sp = 0
	return nil
}

func (p *interp) doFlex1() error {
	if p.sp < 11 {
		return fmt.Errorf("flex1 needs 11 operands")
	}
	s := p.stack[:11]
	dx := s[0] + s[2] + s[4] + s[6] + s[8]
	dy := s[1] + s[3] + s[5] + s[7] + s[9]
	var d6x, d6y float64
	if math.Abs(dx) > math.Abs(dy) {
		d6x = s[10]
		d6y = -dy
	} else {
		d6x = -dx
		d6y = s[10]
	}
	p.curveTo(s[0], s[1], s[2], s[3], s[4], s[5])
	p.curveTo(s[6], s[7], s[8], s[9], d6x, d6y)
	p.sp = 0
	return nil
}
