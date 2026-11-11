package annotate

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
)

// Render draws base and doc's operations, cropped to crop, and returns the
// result. A zero crop means the whole document. The base may be nil, which
// produces a transparent annotation layer.
func Render(base image.Image, doc *Doc, crop image.Rectangle) *image.RGBA {
	if doc == nil {
		if crop.Empty() {
			return image.NewRGBA(image.Rect(0, 0, 1, 1))
		}
		return image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	}
	if crop.Empty() {
		crop = image.Rect(0, 0, doc.Width, doc.Height)
	}
	crop = crop.Intersect(image.Rect(0, 0, doc.Width, doc.Height))
	if crop.Empty() {
		crop = image.Rect(0, 0, doc.Width, doc.Height)
	}
	dst := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	if base != nil {
		draw.Draw(dst, dst.Bounds(), base, crop.Min, draw.Src)
	}
	c := &canvas{img: dst, ox: crop.Min.X, oy: crop.Min.Y}
	for _, op := range doc.ops {
		c.draw(op)
	}
	return dst
}

type canvas struct {
	img    *image.RGBA
	ox, oy int
}

func (c *canvas) draw(op Op) {
	switch op.Tool {
	case ToolRect:
		c.rect(op)
	case ToolEllipse:
		c.ellipse(op)
	case ToolArrow:
		c.arrow(op)
	case ToolLine:
		c.line(Point{X: op.Start.X - float32(c.ox), Y: op.Start.Y - float32(c.oy)},
			Point{X: op.End.X - float32(c.ox), Y: op.End.Y - float32(c.oy)}, opWidth(op, 3), op.Color)
	case ToolPen:
		c.pen(op, false)
	case ToolHighlight:
		c.pen(op, true)
	case ToolText:
		size := op.FontSize
		if size <= 0 {
			size = op.Width
		}
		if size <= 0 {
			size = 16
		}
		drawText(c.img, op.Text, Point{X: op.Start.X - float32(c.ox), Y: op.Start.Y - float32(c.oy)}, size, op.Color)
	case ToolMosaic:
		c.mosaic(op)
	case ToolNumber:
		c.number(op)
	}
}

func opWidth(op Op, def float32) float32 {
	if op.Width > 0 {
		return op.Width
	}
	return def
}

func (c *canvas) rect(op Op) {
	r := normalize(op.Start, op.End)
	x0 := float32(r.Min.X) - float32(c.ox)
	y0 := float32(r.Min.Y) - float32(c.oy)
	x1 := float32(r.Max.X) - float32(c.ox)
	y1 := float32(r.Max.Y) - float32(c.oy)
	w := op.Width
	if w <= 0 {
		w = 2
	}
	if op.Filled {
		c.fillRect(x0, y0, x1, y1, op.Color)
		return
	}
	half := w / 2
	// Four bands keep the outline from erasing what lies under the shape.
	c.fillRect(x0-half, y0-half, x1+half, y0+half, op.Color)
	c.fillRect(x0-half, y1-half, x1+half, y1+half, op.Color)
	c.fillRect(x0-half, y0+half, x0+half, y1-half, op.Color)
	c.fillRect(x1-half, y0+half, x1+half, y1-half, op.Color)
}

// fillRect blends a solid rectangle with anti-aliased edges. clearRect is
// used by outlined shapes to hollow the inside without touching pixels
// outside the rectangle.
func (c *canvas) fillRect(x0, y0, x1, y1 float32, col color.RGBA) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	ix0 := int(math.Floor(float64(x0)))
	ix1 := int(math.Ceil(float64(x1)))
	iy0 := int(math.Floor(float64(y0)))
	iy1 := int(math.Ceil(float64(y1)))
	for y := iy0; y < iy1; y++ {
		cy := coverage(float32(y), float32(y+1), y0, y1)
		if cy <= 0 {
			continue
		}
		for x := ix0; x < ix1; x++ {
			cx := coverage(float32(x), float32(x+1), x0, x1)
			if cx <= 0 {
				continue
			}
			c.blend(x, y, col, cx*cy)
		}
	}
}

func (c *canvas) ellipse(op Op) {
	r := normalize(op.Start, op.End)
	cx := float32(r.Min.X+r.Max.X) / 2
	cy := float32(r.Min.Y+r.Max.Y) / 2
	a := float32(r.Dx()) / 2
	b := float32(r.Dy()) / 2
	if a < 1 || b < 1 {
		return
	}
	w := op.Width
	if w <= 0 {
		w = 2
	}
	ix0 := int(math.Floor(float64(r.Min.X))) - 1
	ix1 := int(math.Ceil(float64(r.Max.X))) + 1
	iy0 := int(math.Floor(float64(r.Min.Y))) - 1
	iy1 := int(math.Ceil(float64(r.Max.Y))) + 1
	for y := iy0; y < iy1; y++ {
		py := float32(y) + 0.5 - cy
		for x := ix0; x < ix1; x++ {
			px := float32(x) + 0.5 - cx
			q := (px*px)/(a*a) + (py*py)/(b*b)
			if op.Filled {
				if q <= 1.05 {
					cov := clamp((1.05-q)/0.10, 0, 1)
					c.blend(x-int(c.ox), y-int(c.oy), op.Color, cov)
				}
				continue
			}
			dist := float32(math.Abs(math.Sqrt(float64(q))-1)) * min(a, b)
			cov := clamp(w/2+0.5-dist, 0, 1)
			if cov > 0 {
				c.blend(x-int(c.ox), y-int(c.oy), op.Color, cov)
			}
		}
	}
}

func (c *canvas) line(a, b Point, w float32, col color.RGBA) {
	dx, dy := b.X-a.X, b.Y-a.Y
	length2 := dx*dx + dy*dy
	if length2 < 0.0001 {
		c.disc(a, w/2, col, 1)
		return
	}
	half := w / 2
	minX := int(math.Floor(float64(min(a.X, b.X) - half - 1)))
	maxX := int(math.Ceil(float64(max(a.X, b.X) + half + 1)))
	minY := int(math.Floor(float64(min(a.Y, b.Y) - half - 1)))
	maxY := int(math.Ceil(float64(max(a.Y, b.Y) + half + 1)))
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			t := ((px-a.X)*dx + (py-a.Y)*dy) / length2
			t = clamp(t, 0, 1)
			nx := a.X + t*dx
			ny := a.Y + t*dy
			d := float32(math.Hypot(float64(px-nx), float64(py-ny)))
			cov := clamp(half+0.5-d, 0, 1)
			if cov > 0 {
				c.blend(x-int(c.ox), y-int(c.oy), col, cov)
			}
		}
	}
}

func (c *canvas) disc(center Point, radius float32, col color.RGBA, alpha float32) {
	if radius <= 0 {
		return
	}
	minX := int(math.Floor(float64(center.X - radius - 1)))
	maxX := int(math.Ceil(float64(center.X + radius + 1)))
	minY := int(math.Floor(float64(center.Y - radius - 1)))
	maxY := int(math.Ceil(float64(center.Y + radius + 1)))
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px := float32(x) + 0.5
			py := float32(y) + 0.5
			d := float32(math.Hypot(float64(px-center.X), float64(py-center.Y)))
			cov := clamp(radius+0.5-d, 0, 1) * alpha
			if cov > 0 {
				c.blend(x-int(c.ox), y-int(c.oy), col, cov)
			}
		}
	}
}

func (c *canvas) arrow(op Op) {
	a := Point{X: op.Start.X - float32(c.ox), Y: op.Start.Y - float32(c.oy)}
	b := Point{X: op.End.X - float32(c.ox), Y: op.End.Y - float32(c.oy)}
	w := op.Width
	if w <= 0 {
		w = 3
	}
	length := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
	if length < 1 {
		return
	}
	head := min(max(w*4, 10), length*0.4)
	ux := (b.X - a.X) / length
	uy := (b.Y - a.Y) / length
	base := Point{X: b.X - ux*head, Y: b.Y - uy*head}
	half := head * 0.42
	c.line(a, base, w, op.Color)
	p1 := Point{X: base.X - uy*half, Y: base.Y + ux*half}
	p2 := Point{X: base.X + uy*half, Y: base.Y - ux*half}
	c.triangle(b, p1, p2, op.Color)
}

func (c *canvas) triangle(a, b, d Point, col color.RGBA) {
	minX := int(math.Floor(float64(min(a.X, min(b.X, d.X))))) - 1
	maxX := int(math.Ceil(float64(max(a.X, max(b.X, d.X))))) + 1
	minY := int(math.Floor(float64(min(a.Y, min(b.Y, d.Y))))) - 1
	maxY := int(math.Ceil(float64(max(a.Y, max(b.Y, d.Y))))) + 1
	area := edge(a, b, d)
	if math.Abs(float64(area)) < 0.001 {
		return
	}
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			p := Point{X: float32(x) + 0.5, Y: float32(y) + 0.5}
			w0 := edge(b, d, p) / area
			w1 := edge(d, a, p) / area
			w2 := edge(a, b, p) / area
			if w0 < -0.02 || w1 < -0.02 || w2 < -0.02 {
				continue
			}
			cov := clamp(min(w0, min(w1, w2))/0.05+1, 0, 1)
			c.blend(x-int(c.ox), y-int(c.oy), col, cov)
		}
	}
}

func edge(a, b, p Point) float32 {
	return (p.X-a.X)*(b.Y-a.Y) - (p.Y-a.Y)*(b.X-a.X)
}

func (c *canvas) pen(op Op, highlight bool) {
	col := op.Color
	if highlight {
		col.A = 90
	}
	w := op.Width
	if w <= 0 {
		w = 3
	}
	if highlight {
		w = max(w, 12)
	}
	pts := op.Points
	if len(pts) == 0 {
		pts = []Point{op.Start, op.End}
	}
	if len(pts) == 1 {
		c.disc(Point{X: pts[0].X - float32(c.ox), Y: pts[0].Y - float32(c.oy)}, w/2, col, 1)
		return
	}
	for i := 1; i < len(pts); i++ {
		a := Point{X: pts[i-1].X - float32(c.ox), Y: pts[i-1].Y - float32(c.oy)}
		b := Point{X: pts[i].X - float32(c.ox), Y: pts[i].Y - float32(c.oy)}
		c.line(a, b, w, col)
	}
	if highlight {
		// Round joins keep the marker from showing notches.
		for _, p := range pts {
			c.disc(Point{X: p.X - float32(c.ox), Y: p.Y - float32(c.oy)}, w/2, col, 1)
		}
	}
}

func (c *canvas) mosaic(op Op) {
	r := normalize(op.Start, op.End)
	crop := c.img.Bounds()
	x0 := max(crop.Min.X, int(math.Floor(float64(r.Min.X)))-c.ox)
	x1 := min(crop.Max.X, int(math.Ceil(float64(r.Max.X)))-c.ox)
	y0 := max(crop.Min.Y, int(math.Floor(float64(r.Min.Y)))-c.oy)
	y1 := min(crop.Max.Y, int(math.Ceil(float64(r.Max.Y)))-c.oy)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	block := int(op.Width)
	if block < 4 {
		block = 12
	}
	for by := y0; by < y1; by += block {
		for bx := x0; bx < x1; bx += block {
			ex := min(bx+block, x1)
			ey := min(by+block, y1)
			var sr, sg, sb, sa, n uint64
			for y := by; y < ey; y++ {
				for x := bx; x < ex; x++ {
					i := c.img.PixOffset(x, y)
					sr += uint64(c.img.Pix[i+0])
					sg += uint64(c.img.Pix[i+1])
					sb += uint64(c.img.Pix[i+2])
					sa += uint64(c.img.Pix[i+3])
					n++
				}
			}
			if n == 0 {
				continue
			}
			avg := color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)}
			for y := by; y < ey; y++ {
				for x := bx; x < ex; x++ {
					i := c.img.PixOffset(x, y)
					c.img.Pix[i+0] = avg.R
					c.img.Pix[i+1] = avg.G
					c.img.Pix[i+2] = avg.B
					c.img.Pix[i+3] = avg.A
				}
			}
		}
	}
}

func (c *canvas) number(op Op) {
	size := op.Width
	if size <= 0 {
		size = 24
	}
	center := Point{X: op.Start.X - float32(c.ox), Y: op.Start.Y - float32(c.oy)}
	c.disc(center, size/2, op.Color, 1)
	label := strconv.Itoa(op.Number)
	if label == "" || op.Number == 0 {
		label = "1"
	}
	faceSize := size * 0.62
	tw, th := Measure(label, faceSize)
	_ = th
	tx := center.X - tw/2
	ty := center.Y - faceSize*0.62
	drawText(c.img, label, Point{X: tx, Y: ty}, faceSize, color.RGBA{R: 255, G: 255, B: 255, A: 255})
}

func (c *canvas) blend(x, y int, col color.RGBA, alpha float32) {
	if alpha <= 0 {
		return
	}
	b := c.img.Bounds()
	if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
		return
	}
	sa := clamp(alpha, 0, 1) * float32(col.A) / 255
	if sa <= 0 {
		return
	}
	i := c.img.PixOffset(x, y)
	da := float32(c.img.Pix[i+3]) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		c.img.Pix[i+0], c.img.Pix[i+1], c.img.Pix[i+2], c.img.Pix[i+3] = 0, 0, 0, 0
		return
	}
	blend := func(sc, dc uint8) uint8 {
		v := (float32(sc)*sa + float32(dc)*da*(1-sa)) / outA
		return uint8(clamp(v, 0, 255) + 0.5)
	}
	c.img.Pix[i+0] = blend(col.R, c.img.Pix[i+0])
	c.img.Pix[i+1] = blend(col.G, c.img.Pix[i+1])
	c.img.Pix[i+2] = blend(col.B, c.img.Pix[i+2])
	c.img.Pix[i+3] = uint8(clamp(outA*255, 0, 255) + 0.5)
}

func normalize(a, b Point) image.Rectangle {
	x0, x1 := a.X, b.X
	y0, y1 := a.Y, b.Y
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return image.Rect(int(math.Floor(float64(x0))), int(math.Floor(float64(y0))),
		int(math.Ceil(float64(x1))), int(math.Ceil(float64(y1))))
}

func coverage(lo, hi, edgeLo, edgeHi float32) float32 {
	if hi <= edgeLo || lo >= edgeHi {
		return 0
	}
	w := min(hi, edgeHi) - max(lo, edgeLo)
	return clamp(w, 0, 1)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
