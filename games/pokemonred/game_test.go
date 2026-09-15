package pokemonred

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/maestroi/gamevision/pkg/game"
)

func testROM() []byte {
	rom := make([]byte, 32*1024)
	rom[0x0100] = 0xC3
	rom[0x0101] = 0x00
	rom[0x0102] = 0x01
	copy(rom[0x0134:0x0144], []byte("GVTEST"))
	return rom
}

func TestObserveScaledPNG(t *testing.T) {
	g, err := Open(Config{ROMBytes: testROM(), Scale: 2, HoldFrames: 1, SettleFrames: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	obs, err := g.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if obs.NativeWidth != 160 || obs.NativeHeight != 144 {
		t.Fatalf("native %dx%d", obs.NativeWidth, obs.NativeHeight)
	}
	if obs.Width != 320 || obs.Height != 288 {
		t.Fatalf("scaled %dx%d", obs.Width, obs.Height)
	}
	if len(obs.Image) == 0 || !bytes.HasPrefix(obs.Image, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("expected PNG")
	}
}

func TestApplyRejectsUnknownAction(t *testing.T) {
	g, err := Open(Config{ROMBytes: testROM(), HoldFrames: 1, SettleFrames: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if err := g.Apply(context.Background(), game.Action{Name: "JUMP"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyWAITAdvancesFrames(t *testing.T) {
	g, err := Open(Config{ROMBytes: testROM(), HoldFrames: 2, SettleFrames: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	before := g.FrameCount()
	if err := g.Apply(context.Background(), game.Action{Name: ActionWait}); err != nil {
		t.Fatal(err)
	}
	got := g.FrameCount() - before
	if got != 5 {
		t.Fatalf("advanced %d frames, want 5", got)
	}
}

func TestSaveLoadState(t *testing.T) {
	g, err := Open(Config{ROMBytes: testROM(), HoldFrames: 1, SettleFrames: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	_ = g.Apply(context.Background(), game.Action{Name: ActionA})
	st, err := g.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	obs1, err := g.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h1 := sha256.Sum256(obs1.Image)
	if err := g.Apply(context.Background(), game.Action{Name: ActionRight}); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadState(st); err != nil {
		t.Fatal(err)
	}
	obs2, err := g.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h2 := sha256.Sum256(obs2.Image)
	if h1 != h2 {
		t.Fatal("loaded state did not restore the observation")
	}
}
