package canvas

import (
	"fmt"
	"image/color"
	"io/ioutil"
	"math"
	"os/exec"
	"reflect"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
)

// FontStyle defines the font style to be used for the font.
type FontStyle int

// see FontStyle
const (
	FontRegular    FontStyle = 0 // 400
	FontItalic     FontStyle = 1
	FontExtraLight FontStyle = 2 << iota // 100
	FontLight                            // 200
	FontBook                             // 300
	FontMedium                           // 500
	FontSemibold                         // 600
	FontBold                             // 700
	FontBlack                            // 800
	FontExtraBlack                       // 900
)

// FontVariant defines the font variant to be used for the font, such as subscript or smallcaps.
type FontVariant int

// see FontVariant
const (
	FontNormal FontVariant = 2 << iota
	FontSubscript
	FontSuperscript
	FontSmallcaps
)

// FontFamily contains a family of fonts (bold, italic, ...). Selecting an italic style will pick the native italic font or use faux italic if not present.
type FontFamily struct {
	name    string
	fonts   map[FontStyle]*Font
	options TypographicOptions
}

// NewFontFamily returns a new FontFamily.
func NewFontFamily(name string) *FontFamily {
	return &FontFamily{
		name:  name,
		fonts: map[FontStyle]*Font{},
	}
}

// LoadLocalFont loads a font from the system fonts location.
func (family *FontFamily) LoadLocalFont(name string, style FontStyle) error {
	match := name
	if style&FontItalic == FontItalic {
		match += ":italic"
	}
	if style&FontExtraLight == FontExtraLight {
		match += ":weight=40"
	} else if style&FontLight == FontLight {
		match += ":weight=50"
	} else if style&FontBook == FontBook {
		match += ":weight=75"
	} else if style&FontMedium == FontMedium {
		match += ":weight=100"
	} else if style&FontSemibold == FontSemibold {
		match += ":weight=180"
	} else if style&FontBold == FontBold {
		match += ":weight=200"
	} else if style&FontBlack == FontBlack {
		match += ":weight=205"
	} else if style&FontExtraBlack == FontExtraBlack {
		match += ":weight=210"
	}
	b, err := exec.Command("fc-match", "--format=%{file}", match).Output()
	if err != nil {
		return err
	}
	return family.LoadFontFile(string(b), style)
}

// LoadFontFile loads a font from a file.
func (family *FontFamily) LoadFontFile(filename string, style FontStyle) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to load font file '%s': %w", filename, err)
	}
	return family.LoadFont(b, style)
}

// LoadFont loads a font from memory.
func (family *FontFamily) LoadFont(b []byte, style FontStyle) error {
	font, err := parseFont(family.name, b)
	if err != nil {
		return err
	}
	font.Use(family.options)
	family.fonts[style] = font
	return nil
}

// Use specifies which typographic options shall be used, ie. whether to use common typographic substitutions and which ligatures classes to use.
func (family *FontFamily) Use(options TypographicOptions) {
	family.options = options
	for _, font := range family.fonts {
		font.Use(options)
	}
}

// Face gets the font face given by the font size (in pt).
func (family *FontFamily) Face(size float64, col color.Color, style FontStyle, variant FontVariant, deco ...FontDecorator) FontFace {
	size *= mmPerPt

	scale := 1.0
	voffset := 0.0
	fauxItalic := 0.0
	fauxBold := 0.0

	font := family.fonts[style]
	if font == nil {
		font = family.fonts[FontRegular]
		if font == nil {
			panic("requested font style not found")
		}
		if style&FontItalic != 0 {
			fauxItalic = 0.3
		}
		if style&FontExtraLight == FontExtraLight {
			fauxBold = -0.02
		} else if style&FontLight == FontLight {
			fauxBold = -0.01
		} else if style&FontBook == FontBook {
			fauxBold = -0.005
		} else if style&FontMedium == FontMedium {
			fauxBold = 0.005
		} else if style&FontSemibold == FontSemibold {
			fauxBold = 0.01
		} else if style&FontBold == FontBold {
			fauxBold = 0.02
		} else if style&FontBlack == FontBlack {
			fauxBold = 0.03
		} else if style&FontExtraBlack == FontExtraBlack {
			fauxBold = 0.04
		}
	}

	// TODO: use subscript/superscript size info from SFNT OS/2 table
	if variant&FontSubscript != 0 || variant&FontSuperscript != 0 {
		scale = 0.583
		fauxBold += 0.02
		if variant&FontSubscript != 0 {
			voffset = -0.33 * size
		} else {
			voffset = 0.33 * size
		}
	}

	r, g, b, a := col.RGBA()
	return FontFace{
		family:     family,
		Font:       font,
		Size:       size,
		Style:      style,
		Variant:    variant,
		Color:      color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)},
		deco:       deco,
		Scale:      scale,
		Voffset:    voffset,
		FauxItalic: fauxItalic,
		FauxBold:   fauxBold * size * scale,
	}
}

// FontFace defines a font face from a given font. It allows setting the font size, its color, faux styles and font decorations.
type FontFace struct {
	family *FontFamily
	Font   *Font

	Size    float64
	Style   FontStyle
	Variant FontVariant
	Color   color.RGBA
	deco    []FontDecorator

	Scale, Voffset, FauxBold, FauxItalic float64 // consequences of font style and variant
}

// Equals returns true when two font face are equal. In particular this allows two adjacent text spans that use the same decoration to allow the decoration to span both elements instead of two separately.
func (ff FontFace) Equals(other FontFace) bool {
	return ff.Font == other.Font && ff.Size == other.Size && ff.Style == other.Style && ff.Variant == other.Variant && ff.Color == other.Color && reflect.DeepEqual(ff.deco, other.deco)
}

// Name returns the name of the underlying font
func (ff FontFace) Name() string {
	return ff.Font.name
}

// Metrics returns the font metrics. See https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png for an explanation of the different metrics.
func (ff FontFace) Metrics() FontMetrics {
	m := ff.Font.Metrics(ff.Size * ff.Scale)
	return FontMetrics{
		LineHeight: math.Abs(m.LineHeight),
		Ascent:     math.Abs(m.Ascent),
		Descent:    math.Abs(m.Descent),
		XHeight:    math.Abs(m.XHeight),
		CapHeight:  math.Abs(m.CapHeight),
	}
}

// Kerning returns the eventual kerning between two runes in mm (ie. the adjustment on the advance).
func (ff FontFace) Kerning(rPrev, rNext rune) float64 {
	k, _ := ff.Font.Kerning(rPrev, rNext, ff.Size*ff.Scale)
	return k
}

// TextWidth returns the width of a given string in mm.
func (ff FontFace) TextWidth(s string) float64 {
	buffer := &sfnt.Buffer{}
	w := 0.0
	var prevIndex sfnt.GlyphIndex
	for i, r := range s {
		index, err := ff.Font.sfnt.GlyphIndex(buffer, r)
		if err != nil {
			continue
		}

		if i != 0 {
			kern, err := ff.Font.sfnt.Kern(buffer, prevIndex, index, toI26_6(ff.Size*ff.Scale), font.HintingNone)
			if err == nil {
				w += fromI26_6(kern)
			}
		}
		advance, err := ff.Font.sfnt.GlyphAdvance(buffer, index, toI26_6(ff.Size*ff.Scale), font.HintingNone)
		if err == nil {
			w += fromI26_6(advance)
		}
		prevIndex = index
	}
	return w
}

// Decorate will return a path from the decorations specified in the FontFace over a given width in mm.
func (ff FontFace) Decorate(width float64) *Path {
	p := &Path{}
	if ff.deco != nil {
		for _, deco := range ff.deco {
			p = p.Append(deco.Decorate(ff, width))
		}
	}
	return p
}

// ToPath converts a string to a path and also returns its advance in mm.
func (ff FontFace) ToPath(s string) (*Path, float64) {
	buffer := &sfnt.Buffer{}
	p := &Path{}
	x := 0.0
	var prevIndex sfnt.GlyphIndex
	for i, r := range s {
		index, err := ff.Font.sfnt.GlyphIndex(buffer, r)
		if err != nil {
			return p, 0.0
		}

		segments, err := ff.Font.sfnt.LoadGlyph(buffer, index, toI26_6(ff.Size*ff.Scale), nil)
		if err != nil {
			return p, 0.0
		}

		if i != 0 {
			kern, err := ff.Font.sfnt.Kern(buffer, prevIndex, index, toI26_6(ff.Size*ff.Scale), font.HintingNone)
			if err == nil {
				x += fromI26_6(kern)
			}
		}

		var start0, end Point
		for i, segment := range segments {
			switch segment.Op {
			case sfnt.SegmentOpMoveTo:
				if i != 0 && start0.Equals(end) {
					p.Close()
				}
				end = fromP26_6(segment.Args[0])
				end.X += ff.FauxItalic * -end.Y
				p.MoveTo(x+end.X, ff.Voffset-end.Y)
				start0 = end
			case sfnt.SegmentOpLineTo:
				end = fromP26_6(segment.Args[0])
				end.X += ff.FauxItalic * -end.Y
				p.LineTo(x+end.X, ff.Voffset-end.Y)
			case sfnt.SegmentOpQuadTo:
				cp := fromP26_6(segment.Args[0])
				end = fromP26_6(segment.Args[1])
				cp.X += ff.FauxItalic * -cp.Y
				end.X += ff.FauxItalic * -end.Y
				p.QuadTo(x+cp.X, ff.Voffset-cp.Y, x+end.X, ff.Voffset-end.Y)
			case sfnt.SegmentOpCubeTo:
				cp1 := fromP26_6(segment.Args[0])
				cp2 := fromP26_6(segment.Args[1])
				end = fromP26_6(segment.Args[2])
				cp1.X += ff.FauxItalic * -cp1.Y
				cp2.X += ff.FauxItalic * -cp2.Y
				end.X += ff.FauxItalic * -end.Y
				p.CubeTo(x+cp1.X, ff.Voffset-cp1.Y, x+cp2.X, ff.Voffset-cp2.Y, x+end.X, ff.Voffset-end.Y)
			}
		}
		if !p.Empty() && start0.Equals(end) {
			p.Close()
		}
		if ff.FauxBold != 0.0 {
			p = p.Offset(ff.FauxBold, NonZero)
		}

		advance, err := ff.Font.sfnt.GlyphAdvance(buffer, index, toI26_6(ff.Size*ff.Scale), font.HintingNone)
		if err == nil {
			x += fromI26_6(advance)
		}
		prevIndex = index
	}
	return p, x
}

func (ff FontFace) Boldness() int {
	boldness := 400
	if ff.Style&FontExtraLight == FontExtraLight {
		boldness = 100
	} else if ff.Style&FontLight == FontLight {
		boldness = 200
	} else if ff.Style&FontBook == FontBook {
		boldness = 300
	} else if ff.Style&FontMedium == FontMedium {
		boldness = 500
	} else if ff.Style&FontSemibold == FontSemibold {
		boldness = 600
	} else if ff.Style&FontBold == FontBold {
		boldness = 700
	} else if ff.Style&FontBlack == FontBlack {
		boldness = 800
	} else if ff.Style&FontExtraBlack == FontExtraBlack {
		boldness = 900
	}
	if ff.Variant&FontSubscript != 0 || ff.Variant&FontSuperscript != 0 {
		boldness += 300
		if 1000 < boldness {
			boldness = 1000
		}
	}
	return boldness
}

////////////////////////////////////////////////////////////////

// FontDecorator is an interface that returns a path given a font face and a width in mm.
type FontDecorator interface {
	Decorate(FontFace, float64) *Path
}

const underlineDistance = 0.15
const underlineThickness = 0.075

// FontUnderline is a font decoration that draws a line under the text at the base line.
var FontUnderline FontDecorator = underline{}

type underline struct{}

func (underline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	y := -ff.Size * underlineDistance

	p := &Path{}
	p.MoveTo(0.0, y)
	p.LineTo(w, y)
	return p.Stroke(r, ButtCap, BevelJoin)
}

// FontOverline is a font decoration that draws a line over the text at the X-Height line.
var FontOverline FontDecorator = overline{}

type overline struct{}

func (overline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	y := ff.Metrics().XHeight + ff.Size*underlineDistance

	dx := ff.FauxItalic * y
	w += ff.FauxItalic * y

	p := &Path{}
	p.MoveTo(dx, y)
	p.LineTo(w, y)
	return p.Stroke(r, ButtCap, BevelJoin)
}

// FontStrikethrough is a font decoration that draws a line through the text in the middle between the base and X-Height line.
var FontStrikethrough FontDecorator = strikethrough{}

type strikethrough struct{}

func (strikethrough) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	y := ff.Metrics().XHeight / 2.0

	dx := ff.FauxItalic * y
	w += ff.FauxItalic * y

	p := &Path{}
	p.MoveTo(dx, y)
	p.LineTo(w, y)
	return p.Stroke(r, ButtCap, BevelJoin)
}

// FontDoubleUnderline is a font decoration that draws two lines at the base line.
var FontDoubleUnderline FontDecorator = doubleUnderline{}

type doubleUnderline struct{}

func (doubleUnderline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	y := -ff.Size * underlineDistance * 0.75

	p := &Path{}
	p.MoveTo(0.0, y)
	p.LineTo(w, y)
	p.MoveTo(0.0, y-r*2.0)
	p.LineTo(w, y-r*2.0)
	return p.Stroke(r, ButtCap, BevelJoin)
}

// FontDottedUnderline is a font decoration that draws a dotted line at the base line.
var FontDottedUnderline FontDecorator = dottedUnderline{}

type dottedUnderline struct{}

func (dottedUnderline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness * 0.8
	w -= r

	y := -ff.Size * underlineDistance
	d := 15.0 * underlineThickness
	n := int((w-r)/d) + 1
	d = (w - r) / float64(n-1)

	p := &Path{}
	for i := 0; i < n; i++ {
		p = p.Append(Circle(r).Translate(r+float64(i)*d, y))
	}
	return p
}

// FontDashedUnderline is a font decoration that draws a dashed line at the base line.
var FontDashedUnderline FontDecorator = dashedUnderline{}

type dashedUnderline struct{}

func (dashedUnderline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	y := -ff.Size * underlineDistance
	d := 12.0 * underlineThickness
	n := int(w / (2.0 * d))
	d = w / float64(2*n-1)

	p := &Path{}
	p.MoveTo(0.0, y)
	p.LineTo(w, y)
	p = p.Dash(d).Stroke(r, ButtCap, BevelJoin)
	return p
}

// FontSineUnderline is a font decoration that draws a wavy sine path at the base line.
var FontSineUnderline FontDecorator = sineUnderline{}

type sineUnderline struct{}

func (sineUnderline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	w -= r

	dh := -ff.Size * 0.15
	y := -ff.Size * underlineDistance
	d := 12.0 * underlineThickness
	n := int(0.5 + w/d)
	d = (w - r) / float64(n)

	dx := r
	p := &Path{}
	p.MoveTo(dx, y)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			p.CubeTo(dx+d*0.3642, y, dx+d*0.6358, y+dh, dx+d, y+dh)
		} else {
			p.CubeTo(dx+d*0.3642, y+dh, dx+d*0.6358, y, dx+d, y)
		}
		dx += d
	}
	return p.Stroke(r, RoundCap, RoundJoin)
}

// FontSawtoothUnderline is a font decoration that draws a wavy sawtooth path at the base line.
var FontSawtoothUnderline FontDecorator = sawtoothUnderline{}

type sawtoothUnderline struct{}

func (sawtoothUnderline) Decorate(ff FontFace, w float64) *Path {
	r := ff.Size * underlineThickness
	dx := 0.707 * r
	w -= 2.0 * dx

	dh := -ff.Size * 0.15
	y := -ff.Size * underlineDistance
	d := 8.0 * underlineThickness
	n := int(0.5 + w/d)
	d = w / float64(n)

	p := &Path{}
	p.MoveTo(dx, y)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			p.LineTo(dx+d, y+dh)
		} else {
			p.LineTo(dx+d, y)
		}
		dx += d
	}
	return p.Stroke(r, ButtCap, MiterJoin)
}
