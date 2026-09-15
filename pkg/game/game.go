// Package game defines the reusable observation/action boundary for GameVision.
//
// Core packages should depend on these types instead of a particular emulator
// or title. Game-specific knowledge belongs in a game adapter.
package game

import "context"

// Observation is what an agent is allowed to see. v1 is vision-first: the
// image is the primary state. Adapters must not stuff RAM-derived Pokémon
// (or other title) state into this struct.
type Observation struct {
	// Image is a PNG encoding of the current screen after configured scaling.
	Image []byte
	// Width and Height are the pixel size of Image, not necessarily native.
	Width  int
	Height int
	// NativeWidth and NativeHeight are the unscaled framebuffer size.
	NativeWidth  int
	NativeHeight int
	// Frame is the emulator frame counter when this observation was taken.
	Frame uint64
}

// Action is a named controller input in the current game's action space.
type Action struct {
	Name string
}

// Source says who produced an input. Benchmarks should treat source=human
// sessions as assisted, not autonomous.
type Source string

const (
	SourceAgent Source = "agent"
	SourceHuman Source = "human"
)

// Game is a thin adapter over some emulator or environment.
type Game interface {
	Name() string
	Observe(ctx context.Context) (Observation, error)
	Apply(ctx context.Context, action Action) error
	Actions() []Action
	Close() error
}

// Previewable games can serve a native-resolution PNG for the debug UI
// without using the model-scale observation image.
type Previewable interface {
	PreviewPNG() ([]byte, error)
}

// Stateful games can load and dump emulator save states. States are only
// used to establish a starting scenario; they are not shown to the model.
type Stateful interface {
	LoadState(data []byte) error
	SaveState() ([]byte, error)
}
