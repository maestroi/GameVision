package vision

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestComparePNGNoChange(t *testing.T) {
	img := solidPNG(80, 72, color.RGBA{R: 20, G: 30, B: 40, A: 255})
	d, err := ComparePNG(img, img)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != ChangeNone {
		t.Fatalf("kind=%s delta=%+v", d.Kind, d)
	}
}

func TestComparePNGIgnoresTinyBlink(t *testing.T) {
	before := image.NewRGBA(image.Rect(0, 0, 80, 72))
	after := image.NewRGBA(image.Rect(0, 0, 80, 72))
	fill(before, color.RGBA{R: 30, G: 30, B: 30, A: 255})
	fill(after, color.RGBA{R: 30, G: 30, B: 30, A: 255})
	after.Set(4, 4, color.White)
	d, err := ComparePNG(encodePNG(before), encodePNG(after))
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != ChangeNone {
		t.Fatalf("tiny blink should be ignored: %+v", d)
	}
}

func TestComparePNGDetectsMovementSizedChange(t *testing.T) {
	before := image.NewRGBA(image.Rect(0, 0, 80, 72))
	after := image.NewRGBA(image.Rect(0, 0, 80, 72))
	fill(before, color.Black)
	fill(after, color.Black)
	for y := 20; y < 28; y++ {
		for x := 20; x < 28; x++ {
			before.Set(x, y, color.White)
			after.Set(x+8, y, color.White)
		}
	}
	d, err := ComparePNG(encodePNG(before), encodePNG(after))
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != ChangeSome {
		t.Fatalf("movement should be visible but not major: %+v", d)
	}
}

func TestComparePNGDetectsMajorChange(t *testing.T) {
	before := solidPNG(80, 72, color.Black)
	after := solidPNG(80, 72, color.White)
	d, err := ComparePNG(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != ChangeMajor {
		t.Fatalf("kind=%s delta=%+v", d.Kind, d)
	}
}

func solidPNG(w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(img, c)
	return encodePNG(img)
}

func fill(img *image.RGBA, c color.Color) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func encodePNG(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}
