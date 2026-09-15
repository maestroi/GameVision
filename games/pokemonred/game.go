package pokemonred

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/maestroi/gomeboy/pkg/gomeboy"

	"github.com/maestroi/gamevision/internal/vision"
	"github.com/maestroi/gamevision/pkg/game"
)

const (
	ActionUp     = "UP"
	ActionDown   = "DOWN"
	ActionLeft   = "LEFT"
	ActionRight  = "RIGHT"
	ActionA      = "A"
	ActionB      = "B"
	ActionStart  = "START"
	ActionSelect = "SELECT"
	ActionWait   = "WAIT"
)

// Default action space for Pokémon Red (Game Boy buttons + WAIT).
func Actions() []game.Action {
	return []game.Action{
		{Name: ActionUp},
		{Name: ActionDown},
		{Name: ActionLeft},
		{Name: ActionRight},
		{Name: ActionA},
		{Name: ActionB},
		{Name: ActionStart},
		{Name: ActionSelect},
		{Name: ActionWait},
	}
}

var buttonByName = map[string]gomeboy.Button{
	ActionUp:     gomeboy.ButtonUp,
	ActionDown:   gomeboy.ButtonDown,
	ActionLeft:   gomeboy.ButtonLeft,
	ActionRight:  gomeboy.ButtonRight,
	ActionA:      gomeboy.ButtonA,
	ActionB:      gomeboy.ButtonB,
	ActionStart:  gomeboy.ButtonStart,
	ActionSelect: gomeboy.ButtonSelect,
}

// Config is Pokémon Red / gomeboy adapter configuration.
type Config struct {
	ROM          string
	ROMBytes     []byte
	State        []byte
	StatePath    string
	Scale        int
	HoldFrames   int
	SettleFrames int
	Interval     int
	CGB          bool
}

// Game wraps a headless gomeboy instance. It never reads Pokémon RAM.
type Game struct {
	mu  sync.Mutex
	emu *gomeboy.Emulator
	cfg Config
}

func Open(cfg Config) (*Game, error) {
	if cfg.Scale <= 0 {
		cfg.Scale = 2
	}
	if cfg.HoldFrames <= 0 {
		cfg.HoldFrames = 16
	}
	if cfg.SettleFrames <= 0 {
		cfg.SettleFrames = 12
	}
	opts := []gomeboy.Option{gomeboy.Headless()}
	if cfg.CGB {
		opts = append(opts, gomeboy.WithModel(gomeboy.ModelCGB))
	}
	if len(cfg.ROMBytes) > 0 {
		opts = append(opts, gomeboy.WithROMBytes(cfg.ROMBytes))
	} else if cfg.ROM != "" {
		opts = append(opts, gomeboy.WithROM(cfg.ROM))
	} else {
		return nil, fmt.Errorf("pokemonred: rom is required")
	}
	emu, err := gomeboy.New(opts...)
	if err != nil {
		return nil, err
	}
	g := &Game{emu: emu, cfg: cfg}
	if len(cfg.State) == 0 && cfg.StatePath != "" {
		cfg.State, err = os.ReadFile(cfg.StatePath)
		if err != nil {
			_ = emu.Close()
			return nil, fmt.Errorf("pokemonred: read state: %w", err)
		}
		g.cfg.State = cfg.State
	}
	if len(g.cfg.State) > 0 {
		if err := emu.LoadState(g.cfg.State); err != nil {
			_ = emu.Close()
			return nil, fmt.Errorf("pokemonred: load state: %w", err)
		}
	} else {
		// Produce a first framebuffer so Observe does not return a boot-blank
		// screen before the title sequence has drawn anything.
		emu.StepFrames(1)
	}
	return g, nil
}

func (g *Game) Name() string { return "Pokémon Red" }

func (g *Game) Actions() []game.Action { return Actions() }

func (g *Game) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.Close()
}

func (g *Game) Observe(ctx context.Context) (game.Observation, error) {
	if err := ctx.Err(); err != nil {
		return game.Observation{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	f := g.emu.Frame()
	rgb, w, h, err := vision.ScaleRGB(f.RGB, f.Width, f.Height, g.cfg.Scale)
	if err != nil {
		return game.Observation{}, err
	}
	png, err := vision.EncodePNG(rgb, w, h)
	if err != nil {
		return game.Observation{}, err
	}
	return game.Observation{
		Image:        png,
		Width:        w,
		Height:       h,
		NativeWidth:  f.Width,
		NativeHeight: f.Height,
		Frame:        g.emu.FrameCount(),
	}, nil
}

func (g *Game) Apply(ctx context.Context, action game.Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.applyLocked(action)
}

func (g *Game) applyLocked(action game.Action) error {
	hold := g.cfg.HoldFrames
	settle := g.cfg.SettleFrames
	if action.Name == ActionWait {
		g.emu.StepFrames(hold + settle)
	} else {
		btn, ok := buttonByName[action.Name]
		if !ok {
			return fmt.Errorf("pokemonred: unknown action %q", action.Name)
		}
		g.emu.Press(btn)
		g.emu.StepFrames(hold)
		g.emu.Release(btn)
		g.emu.StepFrames(settle)
	}
	extra := g.cfg.Interval - hold - settle
	if extra > 0 {
		g.emu.StepFrames(extra)
	}
	return nil
}

func (g *Game) PreviewPNG() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.PNG()
}

func (g *Game) LoadState(data []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.LoadState(data)
}

func (g *Game) SaveState() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.SaveState()
}

func (g *Game) FrameCount() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.FrameCount()
}

// ApplyTimed applies an action and reports emulator wall time.
func (g *Game) ApplyTimed(ctx context.Context, action game.Action) (time.Duration, error) {
	start := time.Now()
	err := g.Apply(ctx, action)
	return time.Since(start), err
}
