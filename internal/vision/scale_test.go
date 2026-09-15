package vision

import (
	"bytes"
	"image/png"
	"testing"
)

func TestScaleRGBIdentityCopy(t *testing.T) {
	src := []byte{1, 2, 3, 4, 5, 6}
	got, w, h, err := ScaleRGB(src, 2, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if w != 2 || h != 1 {
		t.Fatalf("size %dx%d", w, h)
	}
	if !bytes.Equal(got, src) {
		t.Fatalf("got %v want %v", got, src)
	}
	got[0] = 9
	if src[0] != 1 {
		t.Fatal("ScaleRGB must copy at scale 1")
	}
}

func TestScaleRGBNearestNeighbor2x(t *testing.T) {
	// 2x1: red, blue
	src := []byte{
		255, 0, 0,
		0, 0, 255,
	}
	got, w, h, err := ScaleRGB(src, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if w != 4 || h != 2 {
		t.Fatalf("size %dx%d, want 4x2", w, h)
	}
	// Each source pixel becomes a 2x2 block.
	want := []byte{
		255, 0, 0, 255, 0, 0, 0, 0, 255, 0, 0, 255,
		255, 0, 0, 255, 0, 0, 0, 0, 255, 0, 0, 255,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEncodePNGRoundTrip(t *testing.T) {
	rgb := []byte{10, 20, 30, 40, 50, 60}
	data, err := EncodePNG(rgb, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 2 || b.Dy() != 1 {
		t.Fatalf("decoded %dx%d", b.Dx(), b.Dy())
	}
	r, g, bl, _ := img.At(0, 0).RGBA()
	if byte(r>>8) != 10 || byte(g>>8) != 20 || byte(bl>>8) != 30 {
		t.Fatalf("pixel0 = %d,%d,%d", r>>8, g>>8, bl>>8)
	}
}
