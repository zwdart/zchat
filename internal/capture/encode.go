package capture

import (
	"bytes"
	"image"
	"image/png"
)

// RGBAFromBGRA copies a top-down BGRA buffer into a new RGBA image. Windows
// GDI returns alpha as zero even when it uses 32 bits per pixel, so the
// alpha channel is forced opaque.
func RGBAFromBGRA(data []byte, stride, width, height int) *image.RGBA {
	if width <= 0 || height <= 0 || stride < width*4 || len(data) < stride*height {
		return image.NewRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		src := data[y*stride:]
		row := dst.Pix[y*dst.Stride:]
		for x := 0; x < width; x++ {
			i := x * 4
			row[i+0] = src[i+2]
			row[i+1] = src[i+1]
			row[i+2] = src[i+0]
			row[i+3] = 255
		}
	}
	return dst
}

// RGBAFromPacked copies a packed BGRA/RGBA-style pixel buffer into an RGBA
// image using masks and the source's byte order. It is used by the X11
// backend, where the server reports its masks and byte order.
func RGBAFromPacked(data []byte, stride, width, height, bytesPerPixel int, byteOrder uint32, redMask, greenMask, blueMask uint32) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	if width <= 0 || height <= 0 || bytesPerPixel < 2 || stride < width*bytesPerPixel || len(data) < stride*height {
		return dst
	}
	maskToShift := func(mask uint32) (uint, uint32) {
		if mask == 0 {
			return 0, 0
		}
		var shift uint
		for mask&1 == 0 {
			mask >>= 1
			shift++
		}
		return shift, mask
	}
	rs, rm := maskToShift(redMask)
	gs, gm := maskToShift(greenMask)
	bs, bm := maskToShift(blueMask)
	if rm == 0 {
		rm, gm, bm = 0xff, 0xff00, 0xff0000
		rs, gs, bs = 16, 8, 0
	}
	scale := func(v, mask uint32) uint8 {
		if mask == 0 {
			return 0
		}
		maxv := mask
		v &= mask
		if maxv == 255 {
			return uint8(v)
		}
		return uint8((v*255 + maxv/2) / maxv)
	}
	for y := 0; y < height; y++ {
		row := dst.Pix[y*dst.Stride:]
		line := data[y*stride:]
		for x := 0; x < width; x++ {
			p := line[x*bytesPerPixel:]
			var px uint32
			if byteOrder == 0 { // LSBFirst
				for i := 0; i < bytesPerPixel; i++ {
					px |= uint32(p[i]) << (8 * i)
				}
			} else {
				for i := 0; i < bytesPerPixel; i++ {
					px = px<<8 | uint32(p[i])
				}
			}
			o := x * 4
			row[o+0] = scale(px>>rs, rm)
			row[o+1] = scale(px>>gs, gm)
			row[o+2] = scale(px>>bs, bm)
			row[o+3] = 255
		}
	}
	return dst
}

// EncodePNG encodes img with the given compression level. BestSpeed is used
// for the clipboard, DefaultCompression for files.
func EncodePNG(img image.Image, level png.CompressionLevel) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CloneRGBA returns an independent copy of src.
func CloneRGBA(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}
