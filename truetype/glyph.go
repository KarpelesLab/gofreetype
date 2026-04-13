// Copyright 2010 The Freetype-Go Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"fmt"

	"github.com/KarpelesLab/gofreetype/cff"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// CFFSegment is a path segment emitted by a CFF glyph load. Coordinates are
// in 26.6 fixed-point pixel units, with Y increasing upwards (same
// convention as TrueType's GlyphBuf.Points).
type CFFSegment struct {
	Op               cff.SegmentOp
	X, Y             fixed.Int26_6
	CX1, CY1         fixed.Int26_6
	CX2, CY2         fixed.Int26_6
}

// TODO: implement VerticalHinting.

// A Point is a co-ordinate pair plus whether it is 'on' a contour or an 'off'
// control point.
type Point struct {
	X, Y fixed.Int26_6
	// The Flags' LSB means whether or not this Point is 'on' the contour.
	// Other bits are reserved for internal use.
	Flags uint32
}

// A GlyphBuf holds a glyph's contours. A GlyphBuf can be re-used to load a
// series of glyphs from a Font.
type GlyphBuf struct {
	// AdvanceWidth is the glyph's advance width.
	AdvanceWidth fixed.Int26_6
	// Bounds is the glyph's bounding box.
	Bounds fixed.Rectangle26_6
	// Points contains all Points from all contours of the glyph. If hinting
	// was used to load a glyph then Unhinted contains those Points before they
	// were hinted, and InFontUnits contains those Points before they were
	// hinted and scaled.
	Points, Unhinted, InFontUnits []Point
	// Ends is the point indexes of the end point of each contour. The length
	// of Ends is the number of contours in the glyph. The i'th contour
	// consists of points Points[Ends[i-1]:Ends[i]], where Ends[-1] is
	// interpreted to mean zero.
	Ends []int

	// Segments is populated instead of Points/Ends when the Font's Kind is
	// FontKindCFF. CFF glyphs use cubic Béziers rather than TrueType's
	// quadratics, so a different representation is needed.
	Segments []CFFSegment

	font    *Font
	scale   fixed.Int26_6
	hinting font.Hinting
	hinter  hinter
	// phantomPoints are the co-ordinates of the synthetic phantom points
	// used for hinting and bounding box calculations.
	phantomPoints [4]Point
	// pp1x is the X co-ordinate of the first phantom point. The '1' is
	// using 1-based indexing; pp1x is almost always phantomPoints[0].X.
	// TODO: eliminate this and consistently use phantomPoints[0].X.
	pp1x fixed.Int26_6
	// metricsSet is whether the glyph's metrics have been set yet. For a
	// compound glyph, a sub-glyph may override the outer glyph's metrics.
	metricsSet bool
	// tmp is a scratch buffer.
	tmp []Point
}

// Flags for decoding a glyph's contours. These flags are documented at
// http://developer.apple.com/fonts/TTRefMan/RM06/Chap6glyf.html.
const (
	flagOnCurve = 1 << iota
	flagXShortVector
	flagYShortVector
	flagRepeat
	flagPositiveXShortVector
	flagPositiveYShortVector

	// The remaining flags are for internal use.
	flagTouchedX
	flagTouchedY
)

// The same flag bits (0x10 and 0x20) are overloaded to have two meanings,
// dependent on the value of the flag{X,Y}ShortVector bits.
const (
	flagThisXIsSame = flagPositiveXShortVector
	flagThisYIsSame = flagPositiveYShortVector
)

// Load loads a glyph's contours from a Font, overwriting any previously loaded
// contours for this GlyphBuf. scale is the number of 26.6 fixed point units in
// 1 em, i is the glyph index, and h is the hinting policy.
func (g *GlyphBuf) Load(f *Font, scale fixed.Int26_6, i Index, h font.Hinting) error {
	g.Points = g.Points[:0]
	g.Unhinted = g.Unhinted[:0]
	g.InFontUnits = g.InFontUnits[:0]
	g.Ends = g.Ends[:0]
	g.font = f
	g.hinting = h
	g.scale = scale
	g.pp1x = 0
	g.phantomPoints = [4]Point{}
	g.metricsSet = false

	g.Segments = g.Segments[:0]

	switch f.kind {
	case FontKindCFF:
		return g.loadCFF(f, scale, i)
	case FontKindCFF2:
		return UnsupportedError("CFF2 glyph loading not yet implemented")
	}

	if h != font.HintingNone {
		if err := g.hinter.init(f, scale); err != nil {
			return err
		}
	}
	if err := g.load(0, i, true); err != nil {
		return err
	}
	// Apply gvar variations, if any. We operate on the post-scale Points
	// because that's where the outline ends up; gvar deltas are in FUnits
	// so we scale them with the same scale/fUnitsPerEm factor the TT loader
	// used.
	g.applyGVarDeltas(f, int(i), scale)
	// TODO: this selection of either g.pp1x or g.phantomPoints[0].X isn't ideal,
	// and should be cleaned up once we have all the testScaling tests passing,
	// plus additional tests for Freetype-Go's bounding boxes matching C Freetype's.
	pp1x := g.pp1x
	if h != font.HintingNone {
		pp1x = g.phantomPoints[0].X
	}
	if pp1x != 0 {
		for i := range g.Points {
			g.Points[i].X -= pp1x
		}
	}

	advanceWidth := g.phantomPoints[1].X - g.phantomPoints[0].X
	if h != font.HintingNone {
		if len(f.hdmx) >= 8 {
			if n := u32(f.hdmx, 4); n > 3+uint32(i) {
				for hdmx := f.hdmx[8:]; uint32(len(hdmx)) >= n; hdmx = hdmx[n:] {
					if fixed.Int26_6(hdmx[0]) == scale>>6 {
						advanceWidth = fixed.Int26_6(hdmx[2+i]) << 6
						break
					}
				}
			}
		}
		advanceWidth = (advanceWidth + 32) &^ 63
	}
	g.AdvanceWidth = advanceWidth

	// Set g.Bounds to the 'control box', which is the bounding box of the
	// Bézier curves' control points. This is easier to calculate, no smaller
	// than and often equal to the tightest possible bounding box of the curves
	// themselves. This approach is what C Freetype does. We can't just scale
	// the nominal bounding box in the glyf data as the hinting process and
	// phantom point adjustment may move points outside of that box.
	if len(g.Points) == 0 {
		g.Bounds = fixed.Rectangle26_6{}
	} else {
		p := g.Points[0]
		g.Bounds.Min.X = p.X
		g.Bounds.Max.X = p.X
		g.Bounds.Min.Y = p.Y
		g.Bounds.Max.Y = p.Y
		for _, p := range g.Points[1:] {
			if g.Bounds.Min.X > p.X {
				g.Bounds.Min.X = p.X
			} else if g.Bounds.Max.X < p.X {
				g.Bounds.Max.X = p.X
			}
			if g.Bounds.Min.Y > p.Y {
				g.Bounds.Min.Y = p.Y
			} else if g.Bounds.Max.Y < p.Y {
				g.Bounds.Max.Y = p.Y
			}
		}
		// Snap the box to the grid, if hinting is on.
		if h != font.HintingNone {
			g.Bounds.Min.X &^= 63
			g.Bounds.Min.Y &^= 63
			g.Bounds.Max.X += 63
			g.Bounds.Max.X &^= 63
			g.Bounds.Max.Y += 63
			g.Bounds.Max.Y &^= 63
		}
	}
	return nil
}

func (g *GlyphBuf) load(recursion uint32, i Index, useMyMetrics bool) (err error) {
	// The recursion limit here is arbitrary, but defends against malformed glyphs.
	if recursion >= 32 {
		return UnsupportedError("excessive compound glyph recursion")
	}
	// Find the relevant slice of g.font.glyf.
	var g0, g1 uint32
	if g.font.locaOffsetFormat == locaOffsetFormatShort {
		g0 = 2 * uint32(u16(g.font.loca, 2*int(i)))
		g1 = 2 * uint32(u16(g.font.loca, 2*int(i)+2))
	} else {
		g0 = u32(g.font.loca, 4*int(i))
		g1 = u32(g.font.loca, 4*int(i)+4)
	}

	// Decode the contour count and nominal bounding box, from the first
	// 10 bytes of the glyf data. boundsYMin and boundsXMax, at offsets 4
	// and 6, are unused.
	glyf, ne, boundsXMin, boundsYMax := []byte(nil), 0, fixed.Int26_6(0), fixed.Int26_6(0)
	if g0+10 <= g1 {
		glyf = g.font.glyf[g0:g1]
		ne = int(int16(u16(glyf, 0)))
		boundsXMin = fixed.Int26_6(int16(u16(glyf, 2)))
		boundsYMax = fixed.Int26_6(int16(u16(glyf, 8)))
	}

	// Create the phantom points.
	uhm, pp1x := g.font.unscaledHMetric(i), fixed.Int26_6(0)
	uvm := g.font.unscaledVMetric(i, boundsYMax)
	g.phantomPoints = [4]Point{
		{X: boundsXMin - uhm.LeftSideBearing},
		{X: boundsXMin - uhm.LeftSideBearing + uhm.AdvanceWidth},
		{X: uhm.AdvanceWidth / 2, Y: boundsYMax + uvm.TopSideBearing},
		{X: uhm.AdvanceWidth / 2, Y: boundsYMax + uvm.TopSideBearing - uvm.AdvanceHeight},
	}
	if len(glyf) == 0 {
		g.addPhantomsAndScale(len(g.Points), len(g.Points), true, true)
		copy(g.phantomPoints[:], g.Points[len(g.Points)-4:])
		g.Points = g.Points[:len(g.Points)-4]
		// TODO: also trim g.InFontUnits and g.Unhinted?
		return nil
	}

	// Load and hint the contours.
	if ne < 0 {
		if ne != -1 {
			// http://developer.apple.com/fonts/TTRefMan/RM06/Chap6glyf.html says that
			// "the values -2, -3, and so forth, are reserved for future use."
			return UnsupportedError("negative number of contours")
		}
		pp1x = g.font.scale(g.scale * (boundsXMin - uhm.LeftSideBearing))
		if err := g.loadCompound(recursion, uhm, i, glyf, useMyMetrics); err != nil {
			return err
		}
	} else {
		np0, ne0 := len(g.Points), len(g.Ends)
		program := g.loadSimple(glyf, ne)
		g.addPhantomsAndScale(np0, np0, true, true)
		pp1x = g.Points[len(g.Points)-4].X
		if g.hinting != font.HintingNone {
			if len(program) != 0 {
				err := g.hinter.run(
					program,
					g.Points[np0:],
					g.Unhinted[np0:],
					g.InFontUnits[np0:],
					g.Ends[ne0:],
				)
				if err != nil {
					return err
				}
			}
			// Drop the four phantom points.
			g.InFontUnits = g.InFontUnits[:len(g.InFontUnits)-4]
			g.Unhinted = g.Unhinted[:len(g.Unhinted)-4]
		}
		if useMyMetrics {
			copy(g.phantomPoints[:], g.Points[len(g.Points)-4:])
		}
		g.Points = g.Points[:len(g.Points)-4]
		if np0 != 0 {
			// The hinting program expects the []Ends values to be indexed
			// relative to the inner glyph, not the outer glyph, so we delay
			// adding np0 until after the hinting program (if any) has run.
			for i := ne0; i < len(g.Ends); i++ {
				g.Ends[i] += np0
			}
		}
	}
	if useMyMetrics && !g.metricsSet {
		g.metricsSet = true
		g.pp1x = pp1x
	}
	return nil
}

// loadOffset is the initial offset for loadSimple and loadCompound. The first
// 10 bytes are the number of contours and the bounding box.
const loadOffset = 10

func (g *GlyphBuf) loadSimple(glyf []byte, ne int) (program []byte) {
	offset := loadOffset
	for i := 0; i < ne; i++ {
		g.Ends = append(g.Ends, 1+int(u16(glyf, offset)))
		offset += 2
	}

	// Note the TrueType hinting instructions.
	instrLen := int(u16(glyf, offset))
	offset += 2
	program = glyf[offset : offset+instrLen]
	offset += instrLen

	if ne == 0 {
		return program
	}

	np0 := len(g.Points)
	np1 := np0 + int(g.Ends[len(g.Ends)-1])

	// Decode the flags.
	for i := np0; i < np1; {
		c := uint32(glyf[offset])
		offset++
		g.Points = append(g.Points, Point{Flags: c})
		i++
		if c&flagRepeat != 0 {
			count := glyf[offset]
			offset++
			for ; count > 0; count-- {
				g.Points = append(g.Points, Point{Flags: c})
				i++
			}
		}
	}

	// Decode the co-ordinates.
	var x int16
	for i := np0; i < np1; i++ {
		f := g.Points[i].Flags
		if f&flagXShortVector != 0 {
			dx := int16(glyf[offset])
			offset++
			if f&flagPositiveXShortVector == 0 {
				x -= dx
			} else {
				x += dx
			}
		} else if f&flagThisXIsSame == 0 {
			x += int16(u16(glyf, offset))
			offset += 2
		}
		g.Points[i].X = fixed.Int26_6(x)
	}
	var y int16
	for i := np0; i < np1; i++ {
		f := g.Points[i].Flags
		if f&flagYShortVector != 0 {
			dy := int16(glyf[offset])
			offset++
			if f&flagPositiveYShortVector == 0 {
				y -= dy
			} else {
				y += dy
			}
		} else if f&flagThisYIsSame == 0 {
			y += int16(u16(glyf, offset))
			offset += 2
		}
		g.Points[i].Y = fixed.Int26_6(y)
	}

	return program
}

func (g *GlyphBuf) loadCompound(recursion uint32, uhm HMetric, i Index,
	glyf []byte, useMyMetrics bool) error {

	// Flags for decoding a compound glyph. These flags are documented at
	// http://developer.apple.com/fonts/TTRefMan/RM06/Chap6glyf.html.
	const (
		flagArg1And2AreWords = 1 << iota
		flagArgsAreXYValues
		flagRoundXYToGrid
		flagWeHaveAScale
		flagUnused
		flagMoreComponents
		flagWeHaveAnXAndYScale
		flagWeHaveATwoByTwo
		flagWeHaveInstructions
		flagUseMyMetrics
		flagOverlapCompound
	)
	np0, ne0 := len(g.Points), len(g.Ends)
	offset := loadOffset
	for {
		flags := u16(glyf, offset)
		component := Index(u16(glyf, offset+2))
		dx, dy, transform, hasTransform := fixed.Int26_6(0), fixed.Int26_6(0), [4]int16{}, false
		if flags&flagArg1And2AreWords != 0 {
			dx = fixed.Int26_6(int16(u16(glyf, offset+4)))
			dy = fixed.Int26_6(int16(u16(glyf, offset+6)))
			offset += 8
		} else {
			dx = fixed.Int26_6(int16(int8(glyf[offset+4])))
			dy = fixed.Int26_6(int16(int8(glyf[offset+5])))
			offset += 6
		}
		if flags&flagArgsAreXYValues == 0 {
			return UnsupportedError("compound glyph transform vector")
		}
		if flags&(flagWeHaveAScale|flagWeHaveAnXAndYScale|flagWeHaveATwoByTwo) != 0 {
			hasTransform = true
			switch {
			case flags&flagWeHaveAScale != 0:
				transform[0] = int16(u16(glyf, offset+0))
				transform[3] = transform[0]
				offset += 2
			case flags&flagWeHaveAnXAndYScale != 0:
				transform[0] = int16(u16(glyf, offset+0))
				transform[3] = int16(u16(glyf, offset+2))
				offset += 4
			case flags&flagWeHaveATwoByTwo != 0:
				transform[0] = int16(u16(glyf, offset+0))
				transform[1] = int16(u16(glyf, offset+2))
				transform[2] = int16(u16(glyf, offset+4))
				transform[3] = int16(u16(glyf, offset+6))
				offset += 8
			}
		}
		savedPP := g.phantomPoints
		np0 := len(g.Points)
		componentUMM := useMyMetrics && (flags&flagUseMyMetrics != 0)
		if err := g.load(recursion+1, component, componentUMM); err != nil {
			return err
		}
		if flags&flagUseMyMetrics == 0 {
			g.phantomPoints = savedPP
		}
		if hasTransform {
			for j := np0; j < len(g.Points); j++ {
				p := &g.Points[j]
				newX := 0 +
					fixed.Int26_6((int64(p.X)*int64(transform[0])+1<<13)>>14) +
					fixed.Int26_6((int64(p.Y)*int64(transform[2])+1<<13)>>14)
				newY := 0 +
					fixed.Int26_6((int64(p.X)*int64(transform[1])+1<<13)>>14) +
					fixed.Int26_6((int64(p.Y)*int64(transform[3])+1<<13)>>14)
				p.X, p.Y = newX, newY
			}
		}
		dx = g.font.scale(g.scale * dx)
		dy = g.font.scale(g.scale * dy)
		if flags&flagRoundXYToGrid != 0 {
			dx = (dx + 32) &^ 63
			dy = (dy + 32) &^ 63
		}
		for j := np0; j < len(g.Points); j++ {
			p := &g.Points[j]
			p.X += dx
			p.Y += dy
		}
		// TODO: also adjust g.InFontUnits and g.Unhinted?
		if flags&flagMoreComponents == 0 {
			break
		}
	}

	instrLen := 0
	if g.hinting != font.HintingNone && offset+2 <= len(glyf) {
		instrLen = int(u16(glyf, offset))
		offset += 2
	}

	g.addPhantomsAndScale(np0, len(g.Points), false, instrLen > 0)
	points, ends := g.Points[np0:], g.Ends[ne0:]
	g.Points = g.Points[:len(g.Points)-4]
	for j := range points {
		points[j].Flags &^= flagTouchedX | flagTouchedY
	}

	if instrLen == 0 {
		if !g.metricsSet {
			copy(g.phantomPoints[:], points[len(points)-4:])
		}
		return nil
	}

	// Hint the compound glyph.
	program := glyf[offset : offset+instrLen]
	// Temporarily adjust the ends to be relative to this compound glyph.
	if np0 != 0 {
		for i := range ends {
			ends[i] -= np0
		}
	}
	// Hinting instructions of a composite glyph completely refer to the
	// (already) hinted subglyphs.
	g.tmp = append(g.tmp[:0], points...)
	if err := g.hinter.run(program, points, g.tmp, g.tmp, ends); err != nil {
		return err
	}
	if np0 != 0 {
		for i := range ends {
			ends[i] += np0
		}
	}
	if !g.metricsSet {
		copy(g.phantomPoints[:], points[len(points)-4:])
	}
	return nil
}

func (g *GlyphBuf) addPhantomsAndScale(np0, np1 int, simple, adjust bool) {
	// Add the four phantom points.
	g.Points = append(g.Points, g.phantomPoints[:]...)
	// Scale the points.
	if simple && g.hinting != font.HintingNone {
		g.InFontUnits = append(g.InFontUnits, g.Points[np1:]...)
	}
	for i := np1; i < len(g.Points); i++ {
		p := &g.Points[i]
		p.X = g.font.scale(g.scale * p.X)
		p.Y = g.font.scale(g.scale * p.Y)
	}
	if g.hinting == font.HintingNone {
		return
	}
	// Round the 1st phantom point to the grid, shifting all other points equally.
	// Note that "all other points" starts from np0, not np1.
	// TODO: delete this adjustment and the np0/np1 distinction, when
	// we update the compatibility tests to C Freetype 2.5.3.
	// See http://git.savannah.gnu.org/cgit/freetype/freetype2.git/commit/?id=05c786d990390a7ca18e62962641dac740bacb06
	if adjust {
		pp1x := g.Points[len(g.Points)-4].X
		if dx := ((pp1x + 32) &^ 63) - pp1x; dx != 0 {
			for i := np0; i < len(g.Points); i++ {
				g.Points[i].X += dx
			}
		}
	}
	if simple {
		g.Unhinted = append(g.Unhinted, g.Points[np1:]...)
	}
	// Round the 2nd and 4th phantom point to the grid.
	p := &g.Points[len(g.Points)-3]
	p.X = (p.X + 32) &^ 63
	p = &g.Points[len(g.Points)-1]
	p.Y = (p.Y + 32) &^ 63
}

// applyGVarDeltas applies gvar point deltas for glyph `gid` to g.Points,
// using the Font's current variation coordinates. FUnit deltas are
// converted to the same scaled space as Points.
func (g *GlyphBuf) applyGVarDeltas(f *Font, gid int, scale fixed.Int26_6) {
	if f.gvarTable == nil || f.variationCoords == nil {
		return
	}
	// All-zero coords = default instance, nothing to do.
	allZero := true
	for _, c := range f.variationCoords {
		if c != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return
	}
	// numOutlinePoints excludes the four phantom points that Load appends.
	numOutlinePoints := len(g.Points) - 4
	if numOutlinePoints < 0 {
		return
	}
	deltas := f.gvarTable.ApplyDeltas(gid, f.variationCoords, numOutlinePoints)
	if deltas == nil {
		return
	}
	// Scale factor to convert FUnit deltas to scaled coords. The TT loader
	// multiplies raw FUnit values by (scale / fUnitsPerEm) when building
	// Points (the multiplication happens inside scale_point(); scale is
	// the em-height in 26.6 pixels).
	if f.fUnitsPerEm == 0 {
		return
	}
	sx := float64(scale) / float64(f.fUnitsPerEm)
	for i := 0; i < len(g.Points) && i < len(deltas); i++ {
		g.Points[i].X += fixed.Int26_6(deltas[i].X * sx)
		g.Points[i].Y += fixed.Int26_6(deltas[i].Y * sx)
	}
}

// loadCFF loads a CFF glyph into g. The output uses g.Segments (cubic-aware)
// rather than g.Points/Ends (TrueType quadratic). Hinting is not applied —
// CFF hinting needs its own interpreter which is a Phase 4 task.
func (g *GlyphBuf) loadCFF(f *Font, scale fixed.Int26_6, i Index) error {
	if f.cffFont == nil {
		return UnsupportedError("CFF font lacks parsed container")
	}
	if int(i) >= len(f.cffFont.CharStrings) {
		return FormatError(fmt.Sprintf("glyph index %d out of range (%d charstrings)", i, len(f.cffFont.CharStrings)))
	}
	glyph, err := f.cffFont.LoadGlyph(int(i))
	if err != nil {
		return err
	}

	// CFF charstring coordinates are in "design units". The FontMatrix maps
	// them to em space; the outer em-to-pixel scaling is `scale / fUnitsPerEm`.
	// For a typical CFF font with FontMatrix = (0.001, 0, 0, 0.001, 0, 0) and
	// head.unitsPerEm = 1000, design units equal font units, so the composed
	// scale is just `scale / fUnitsPerEm`.
	m := f.cffFont.FontMatrix
	// k is design-units-per-em, derived from |FontMatrix[0]| = 1/unitsPerEm.
	invM0 := 1.0
	if m[0] != 0 {
		invM0 = 1.0 / m[0]
	}
	kx := float64(scale) / invM0
	// Y axis: CFF has Y up; we store Y up in GlyphBuf to match TT, and the
	// renderer flips it when drawing.
	ky := kx
	if m[3] != 0 {
		invM3 := 1.0 / m[3]
		ky = float64(scale) / invM3
	}

	toFixed := func(x, y float64) (fixed.Int26_6, fixed.Int26_6) {
		return fixed.Int26_6(x * kx), fixed.Int26_6(y * ky)
	}

	g.Segments = g.Segments[:0]
	for _, s := range glyph.Segments {
		var out CFFSegment
		out.Op = s.Op
		out.X, out.Y = toFixed(s.X, s.Y)
		if s.Op == cff.SegCubicTo {
			out.CX1, out.CY1 = toFixed(s.CX1, s.CY1)
			out.CX2, out.CY2 = toFixed(s.CX2, s.CY2)
		}
		g.Segments = append(g.Segments, out)
	}

	// Advance width: hmtx is shared with TT. Use the font's HMetric.
	h := f.HMetric(scale, i)
	g.AdvanceWidth = h.AdvanceWidth

	// Bounds = control-point bbox of all segments.
	if len(g.Segments) == 0 {
		g.Bounds = fixed.Rectangle26_6{}
	} else {
		s0 := g.Segments[0]
		g.Bounds.Min.X = s0.X
		g.Bounds.Max.X = s0.X
		g.Bounds.Min.Y = s0.Y
		g.Bounds.Max.Y = s0.Y
		extend := func(x, y fixed.Int26_6) {
			if x < g.Bounds.Min.X {
				g.Bounds.Min.X = x
			} else if x > g.Bounds.Max.X {
				g.Bounds.Max.X = x
			}
			if y < g.Bounds.Min.Y {
				g.Bounds.Min.Y = y
			} else if y > g.Bounds.Max.Y {
				g.Bounds.Max.Y = y
			}
		}
		for _, s := range g.Segments[1:] {
			extend(s.X, s.Y)
			if s.Op == cff.SegCubicTo {
				extend(s.CX1, s.CY1)
				extend(s.CX2, s.CY2)
			}
		}
	}
	return nil
}
