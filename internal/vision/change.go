package vision

import (
	"bytes"
	"image"
	_ "image/png"
)

// ChangeKind is a coarse visual consequence of a controller input. It is
// deliberately generic: GameVision does not infer game state from emulator
// memory, only how much the rendered frame changed.
type ChangeKind string

const (
	ChangeUnknown ChangeKind = "unknown"
	ChangeNone    ChangeKind = "no_visual_change"
	ChangeSome    ChangeKind = "visual_change"
	ChangeMajor   ChangeKind = "major_visual_change"
)

// FrameDelta summarizes perceptual difference between two rendered PNGs.
// ChangedFraction is the fraction of sampled pixels whose RGB delta is visibly
// meaningful. MeanDelta is the normalized average RGB difference (0..1).
type FrameDelta struct {
	Kind            ChangeKind
	ChangedFraction float64
	MeanDelta       float64
}

// ComparePNG compares rendered frames while ignoring tiny pixel noise such as
// cursor/sprite blinking. It samples at most about 80x72 points so the result
// is cheap and independent of nearest-neighbor vision scaling.
func ComparePNG(beforePNG, afterPNG []byte) (FrameDelta, error) {
	before, _, err := image.Decode(bytes.NewReader(beforePNG))
	if err != nil {
		return FrameDelta{Kind: ChangeUnknown}, err
	}
	after, _, err := image.Decode(bytes.NewReader(afterPNG))
	if err != nil {
		return FrameDelta{Kind: ChangeUnknown}, err
	}
	bb := before.Bounds()
	ab := after.Bounds()
	if bb.Dx() != ab.Dx() || bb.Dy() != ab.Dy() || bb.Dx() == 0 || bb.Dy() == 0 {
		return FrameDelta{Kind: ChangeMajor, ChangedFraction: 1, MeanDelta: 1}, nil
	}

	stepX := maxInt(1, bb.Dx()/80)
	stepY := maxInt(1, bb.Dy()/72)
	const visibleThreshold = 16.0 / 255.0

	var samples, changed int
	var sum float64
	for y := 0; y < bb.Dy(); y += stepY {
		for x := 0; x < bb.Dx(); x += stepX {
			br, bg, bbv, _ := before.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			ar, ag, abv, _ := after.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			d := (abs16(br, ar) + abs16(bg, ag) + abs16(bbv, abv)) / (3.0 * 65535.0)
			sum += d
			samples++
			if d >= visibleThreshold {
				changed++
			}
		}
	}
	if samples == 0 {
		return FrameDelta{Kind: ChangeUnknown}, nil
	}

	fraction := float64(changed) / float64(samples)
	mean := sum / float64(samples)
	kind := ChangeSome
	// Less than ~1% of the screen changing is usually animation/cursor noise,
	// not evidence that a movement command made progress.
	if fraction < 0.01 && mean < 0.006 {
		kind = ChangeNone
	} else if fraction >= 0.45 || mean >= 0.18 {
		// A transition, battle/menu takeover, or other wholesale scene change.
		kind = ChangeMajor
	}
	return FrameDelta{Kind: kind, ChangedFraction: fraction, MeanDelta: mean}, nil
}

func abs16(a, b uint32) float64 {
	if a >= b {
		return float64(a - b)
	}
	return float64(b - a)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
