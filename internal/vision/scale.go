package vision

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// ScaleRGB nearest-neighbor scales a packed RGB24 buffer by an integer factor.
// scale <= 1 returns a copy of src. This is the only scaler GameVision uses
// for pixel art: bilinear/bicubic blur the tiles the model needs to read.
func ScaleRGB(src []byte, width, height, scale int) ([]byte, int, int, error) {
	if width <= 0 || height <= 0 {
		return nil, 0, 0, fmt.Errorf("vision: invalid size %dx%d", width, height)
	}
	if len(src) < width*height*3 {
		return nil, 0, 0, fmt.Errorf("vision: rgb buffer length %d < %d", len(src), width*height*3)
	}
	if scale <= 1 {
		out := make([]byte, width*height*3)
		copy(out, src[:width*height*3])
		return out, width, height, nil
	}
	dw, dh := width*scale, height*scale
	dst := make([]byte, dw*dh*3)
	for y := 0; y < dh; y++ {
		sy := y / scale
		srcOff := sy * width * 3
		dstOff := y * dw * 3
		for x := 0; x < dw; x++ {
			sx := x / scale
			si := srcOff + sx*3
			di := dstOff + x*3
			dst[di] = src[si]
			dst[di+1] = src[si+1]
			dst[di+2] = src[si+2]
		}
	}
	return dst, dw, dh, nil
}

// EncodePNG writes packed RGB24 as PNG.
func EncodePNG(rgb []byte, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("vision: invalid size %dx%d", width, height)
	}
	if len(rgb) < width*height*3 {
		return nil, fmt.Errorf("vision: rgb buffer length %d < %d", len(rgb), width*height*3)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	pix := img.Pix
	n := width * height
	for i := 0; i < n; i++ {
		s, d := i*3, i*4
		pix[d] = rgb[s]
		pix[d+1] = rgb[s+1]
		pix[d+2] = rgb[s+2]
		pix[d+3] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
