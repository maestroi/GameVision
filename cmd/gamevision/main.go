// Command gamevision runs a vision-first game agent against a local VLM.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/maestroi/gamevision/games/pokemonred"
	"github.com/maestroi/gamevision/internal/agent"
	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/internal/recording"
	"github.com/maestroi/gamevision/internal/runtime"
	"github.com/maestroi/gamevision/internal/scenario"
	"github.com/maestroi/gamevision/internal/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		if err := runBench(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "models" {
		if err := runModels(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type flags struct {
	game, rom, baseURL, model, apiKey, goal string
	state, scenario, httpAddr, sessionRoot  string
	scale, hold, settle, interval, history  int
	maxDecisions                            int
	maxConsecutiveErrors                    int
	timeout                                 time.Duration
	temperature                             float64
	maxTokens                               int
	cgb, paused, saveDec, saveAll           bool
}

func parseFlags() flags {
	var f flags
	flag.StringVar(&f.game, "game", "pokemon-red", "game adapter")
	flag.StringVar(&f.rom, "rom", os.Getenv("POKEMON_RED_ROM"), "path to the ROM")
	flag.StringVar(&f.baseURL, "base-url", envOr("GAMEVISION_BASE_URL", "http://localhost:8002/v1"), "OpenAI-compatible VLM base URL")
	flag.StringVar(&f.model, "model", envOr("GAMEVISION_MODEL", "Qwen3-VL-2B-Instruct"), "model name")
	flag.StringVar(&f.apiKey, "api-key", firstEnv("GAMEVISION_API_KEY", "OPENAI_API_KEY"), "optional bearer token")
	flag.StringVar(&f.goal, "goal", "Start Pokémon Red and progress as far as possible.", "human-readable session goal")
	flag.StringVar(&f.state, "state", "", "optional gomeboy save state to load at start")
	flag.StringVar(&f.scenario, "scenario", "", "scenario JSON (goal, state, max decisions)")
	flag.StringVar(&f.httpAddr, "http", "localhost:8099", "debug UI listen address; empty disables it")
	flag.StringVar(&f.sessionRoot, "session-dir", "sessions", "session output root")
	flag.IntVar(&f.scale, "vision-scale", 2, "nearest-neighbor scale: 1, 2, or 4")
	flag.IntVar(&f.hold, "button-hold-frames", 16, "frames to hold a pressed button (one Gen 1 walk tile is 16)")
	flag.IntVar(&f.settle, "settle-frames", 12, "frames to wait after release")
	flag.IntVar(&f.interval, "decision-interval-frames", 0, "minimum frames per decision; 0 means hold+settle")
	flag.IntVar(&f.history, "history", 8, "recent actions included in the prompt")
	flag.IntVar(&f.maxDecisions, "max-decisions", 0, "stop after N model decisions; 0 is unlimited")
	flag.IntVar(&f.maxConsecutiveErrors, "max-consecutive-errors", 10, "stop after N consecutive model failures; 0 uses 10, negative disables")
	flag.DurationVar(&f.timeout, "timeout", 60*time.Second, "per-request model timeout")
	flag.Float64Var(&f.temperature, "temperature", 0, "model temperature")
	flag.IntVar(&f.maxTokens, "max-tokens", 16, "completion token cap")
	flag.BoolVar(&f.cgb, "cgb", true, "run Pokémon Red on CGB hardware for colorized frames")
	flag.BoolVar(&f.paused, "paused", false, "start with the agent paused")
	flag.BoolVar(&f.saveDec, "save-decision-frames", true, "write PNGs for model observations")
	flag.BoolVar(&f.saveAll, "save-all-frames", false, "write every captured observation frame")
	flag.Parse()
	return f
}

func run() error {
	f := parseFlags()
	scenName := ""
	if f.scenario != "" {
		sc, err := scenario.Load(f.scenario)
		if err != nil {
			return err
		}
		scenName = sc.Name
		if f.goal == "Start Pokémon Red and progress as far as possible." && sc.Goal != "" {
			f.goal = sc.Goal
		}
		if f.state == "" && sc.State != "" {
			f.state = sc.State
		}
		if f.maxDecisions == 0 && sc.MaxDecisions > 0 {
			f.maxDecisions = sc.MaxDecisions
		}
		if sc.Game != "" && f.game == "pokemon-red" {
			f.game = sc.Game
		}
	}
	if f.rom == "" {
		return fmt.Errorf("--rom is required (or set POKEMON_RED_ROM)")
	}
	if f.scale != 1 && f.scale != 2 && f.scale != 4 {
		return fmt.Errorf("--vision-scale must be 1, 2, or 4")
	}

	g, err := openGame(f)
	if err != nil {
		return err
	}
	defer g.Close()

	sess, err := recording.Open(recording.Options{
		Root:               f.sessionRoot,
		Game:               f.game,
		Goal:               f.goal,
		Model:              f.model,
		BaseURL:            f.baseURL,
		VisionScale:        f.scale,
		HoldFrames:         f.hold,
		SettleFrames:       f.settle,
		History:            f.history,
		Scenario:           scenName,
		StatePath:          f.state,
		SaveDecisionFrames: f.saveDec || f.saveAll,
		SaveAllFrames:      f.saveAll,
	})
	if err != nil {
		return err
	}

	client := inference.New(inference.Config{
		BaseURL:     f.baseURL,
		Model:       f.model,
		APIKey:      f.apiKey,
		Timeout:     f.timeout,
		Temperature: f.temperature,
		MaxTokens:   f.maxTokens,
	})
	ag := agent.New(client)
	rt := runtime.New(g, ag, sess, runtime.Config{
		Goal:                 f.goal,
		Model:                f.model,
		History:              f.history,
		MaxDecisions:         f.maxDecisions,
		MaxConsecutiveErrors: f.maxConsecutiveErrors,
		Paused:               f.paused,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if f.httpAddr != "" {
		srv := ui.New(rt, client, sess.Dir())
		addr, hs, err := srv.Listen(f.httpAddr)
		if err != nil {
			return err
		}
		defer func() { _ = hs.Close() }()
		fmt.Printf("watch: http://%s\n", addr)
	}
	fmt.Printf("session: %s\nmodel: %s\ngoal: %s\n", sess.Dir(), f.model, f.goal)

	rt.Start(ctx)
	done := make(chan struct{})
	go func() {
		rt.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		rt.Stop()
		<-done
	case <-done:
	}

	sum := rt.Summary()
	if err := sess.Finish(sum); err != nil {
		return err
	}
	fmt.Printf("\ndecisions: %d\ninvalid: %d\ntimeouts: %d\nmedian latency: %s\np95 latency: %s\nactions/min: %.1f\n",
		sum.Decisions, sum.InvalidOutputs, sum.Timeouts, sum.Median, sum.P95, sum.ActionsPerMin)
	return nil
}

func runBench() error {
	var (
		scenarioPath = flag.String("scenario", "scenarios/pokemon-red/title-screen.json", "scenario JSON")
		rom          = flag.String("rom", os.Getenv("POKEMON_RED_ROM"), "path to the ROM")
		baseURL      = flag.String("base-url", envOr("GAMEVISION_BASE_URL", "http://localhost:8002/v1"), "VLM base URL")
		models       = flag.String("models", "Qwen3-VL-2B-Instruct,Qwen3-VL-4B-Instruct", "comma-separated models")
		apiKey       = flag.String("api-key", firstEnv("GAMEVISION_API_KEY", "OPENAI_API_KEY"), "optional bearer token")
		httpAddr     = flag.String("http", "", "debug UI listen address; empty disables it during bench")
		sessionRoot  = flag.String("session-dir", "sessions", "session output root")
		scale        = flag.Int("vision-scale", 2, "nearest-neighbor scale")
		hold         = flag.Int("button-hold-frames", 16, "hold frames")
		settle       = flag.Int("settle-frames", 12, "settle frames")
		history      = flag.Int("history", 8, "action history")
		timeout      = flag.Duration("timeout", 60*time.Second, "per-request timeout")
		temperature  = flag.Float64("temperature", 0, "temperature")
		maxTokens    = flag.Int("max-tokens", 16, "max tokens")
		swapModels   = flag.Bool("swap-models", true, "unload the current VLM and load each bench model before that run")
		cgb          = flag.Bool("cgb", true, "CGB colorization")
	)
	flag.Parse()

	sc, err := scenario.Load(*scenarioPath)
	if err != nil {
		return err
	}
	if *rom == "" {
		return fmt.Errorf("--rom is required")
	}

	outDir := filepath.Join(*sessionRoot, "bench-"+time.Now().Format("2006-01-02-150405")+"-"+sc.Name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	type row struct {
		Model   string `json:"model"`
		Dir     string `json:"session"`
		Summary any    `json:"summary"`
	}
	var rows []row
	for _, model := range splitModels(*models) {
		fmt.Printf("\n=== %s ===\n", model)
		client := inference.New(inference.Config{
			BaseURL: *baseURL, Model: model, APIKey: *apiKey,
			Timeout: *timeout, Temperature: *temperature, MaxTokens: *maxTokens,
		})
		if *swapModels {
			fmt.Println("switching inference model (unload current, load next)...")
			swapCtx, swapCancel := context.WithTimeout(context.Background(), 15*time.Minute)
			info, err := client.Switch(swapCtx, model)
			swapCancel()
			if err != nil {
				return fmt.Errorf("switch to %s: %w (pass --swap-models=false if the server cannot hot-swap)", model, err)
			}
			fmt.Printf("loaded %s\n", info.ID)
			model = info.ID
		}
		f := flags{
			game:         "pokemon-red",
			rom:          *rom,
			baseURL:      *baseURL,
			model:        model,
			apiKey:       *apiKey,
			goal:         sc.Goal,
			state:        sc.State,
			httpAddr:     *httpAddr,
			sessionRoot:  outDir,
			scale:        *scale,
			hold:         *hold,
			settle:       *settle,
			history:      *history,
			maxDecisions: sc.MaxDecisions,
			timeout:      *timeout,
			temperature:  *temperature,
			maxTokens:    *maxTokens,
			cgb:          *cgb,
			saveDec:      true,
		}
		g, err := openGame(f)
		if err != nil {
			return err
		}
		sess, err := recording.Open(recording.Options{
			Root:               outDir,
			Game:               f.game,
			Goal:               f.goal,
			Model:              model,
			BaseURL:            f.baseURL,
			VisionScale:        f.scale,
			HoldFrames:         f.hold,
			SettleFrames:       f.settle,
			History:            f.history,
			Scenario:           sc.Name,
			StatePath:          f.state,
			SaveDecisionFrames: true,
		})
		if err != nil {
			_ = g.Close()
			return err
		}
		rt := runtime.New(g, agent.New(client), sess, runtime.Config{
			Goal: f.goal, Model: model, History: f.history, MaxDecisions: f.maxDecisions,
		})
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		rt.Start(ctx)
		rt.Wait()
		cancel()
		sum := rt.Summary()
		_ = sess.Finish(sum)
		_ = g.Close()
		fmt.Printf("decisions=%d median=%s p95=%s invalid=%d timeouts=%d\n",
			sum.Decisions, sum.Median, sum.P95, sum.InvalidOutputs, sum.Timeouts)
		rows = append(rows, row{Model: model, Dir: sess.Dir(), Summary: sum})
		if ctx.Err() != nil {
			break
		}
	}
	cmp, _ := json.MarshalIndent(map[string]any{
		"scenario": sc,
		"models":   rows,
		"shared": map[string]any{
			"vision_scale":       *scale,
			"button_hold_frames": *hold,
			"settle_frames":      *settle,
			"temperature":        *temperature,
			"max_tokens":         *maxTokens,
			"max_decisions":      sc.MaxDecisions,
			"goal":               sc.Goal,
		},
	}, "", "  ")
	cmpPath := filepath.Join(outDir, "comparison.json")
	if err := os.WriteFile(cmpPath, append(cmp, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\ncomparison: %s\n", cmpPath)
	return nil
}

func openGame(f flags) (*pokemonred.Game, error) {
	if f.game != "pokemon-red" && f.game != "pokemonred" {
		return nil, fmt.Errorf("unsupported game %q (v1 supports pokemon-red)", f.game)
	}
	return pokemonred.Open(pokemonred.Config{
		ROM:          f.rom,
		StatePath:    f.state,
		Scale:        f.scale,
		HoldFrames:   f.hold,
		SettleFrames: f.settle,
		Interval:     f.interval,
		CGB:          f.cgb,
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func splitModels(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
