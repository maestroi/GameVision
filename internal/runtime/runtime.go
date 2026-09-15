package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/gamevision/internal/agent"
	"github.com/maestroi/gamevision/internal/metrics"
	"github.com/maestroi/gamevision/internal/recording"
	"github.com/maestroi/gamevision/internal/vision"
	"github.com/maestroi/gamevision/pkg/game"
)

type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusPaused  Status = "paused"
	StatusStopped Status = "stopped"
)

// Snapshot is the observational UI/status payload. It contains no RAM state.
type Snapshot struct {
	Status          Status        `json:"status"`
	Game            string        `json:"game"`
	Goal            string        `json:"goal"`
	Subgoal         string        `json:"subgoal,omitempty"`
	Model           string        `json:"model"`
	Decision        int           `json:"decision_number"`
	LastAction      string        `json:"last_action"`
	LastSource      string        `json:"last_source"`
	LastScene       string        `json:"last_scene,omitempty"`
	LastOutcome     string        `json:"last_visual_outcome,omitempty"`
	LastExpected    string        `json:"last_expected,omitempty"`
	LastRepeat      int           `json:"last_repeat,omitempty"`
	LastChangeScore float64       `json:"last_visual_change_score,omitempty"`
	Recent          []string      `json:"recent_actions"`
	LastLatency     time.Duration `json:"last_latency_ns"`
	LastLatencyMS   float64       `json:"last_latency_ms"`
	Runtime         time.Duration `json:"runtime_ns"`
	RuntimeHuman    string        `json:"runtime"`
	Invalid         int           `json:"invalid_outputs"`
	Timeouts        int           `json:"timeouts"`
	RepeatedState   int           `json:"repeated_visual_state"`
	Error           string        `json:"error,omitempty"`
}

type Config struct {
	Goal                 string
	Model                string
	History              int
	MaxDecisions         int
	MaxConsecutiveErrors int
	Paused               bool
}

// Runtime is the closed observe -> decide -> apply -> verify loop.
type Runtime struct {
	game  game.Game
	agent agent.Agent
	sess  *recording.Session
	cfg   Config

	mu           sync.Mutex
	status       Status
	started      time.Time
	decision     int
	history      []game.Action
	subgoal      string
	lastScene    string
	lastOutcome  string
	lastExpected string
	lastRepeat   int
	lastChange   float64
	lastAct      string
	lastSrc      string
	lastLat      time.Duration
	lastErr      string
	invalid      int
	timeouts     int
	repeated     int
	consecErr    int
	tracker      metrics.Tracker
	preview      []byte
	stop         chan struct{}
	stopped      bool
}

func New(g game.Game, a agent.Agent, sess *recording.Session, cfg Config) *Runtime {
	if cfg.History <= 0 {
		cfg.History = 8
	}
	if cfg.MaxConsecutiveErrors == 0 {
		cfg.MaxConsecutiveErrors = 10
	}
	st := StatusRunning
	if cfg.Paused {
		st = StatusPaused
	}
	return &Runtime{
		game:   g,
		agent:  a,
		sess:   sess,
		cfg:    cfg,
		status: st,
		stop:   make(chan struct{}),
	}
}

func (r *Runtime) Start(ctx context.Context) {
	r.mu.Lock()
	r.started = time.Now()
	if r.status == StatusIdle {
		r.status = StatusRunning
	}
	r.mu.Unlock()
	go r.loop(ctx)
}

func (r *Runtime) loop(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		if !r.stopped {
			r.status = StatusStopped
			r.stopped = true
			close(r.stop)
		}
		r.mu.Unlock()
	}()
	for {
		if err := r.waitRunnable(ctx); err != nil {
			return
		}
		r.mu.Lock()
		max := r.cfg.MaxDecisions
		n := r.decision
		stopped := r.status == StatusStopped
		r.mu.Unlock()
		if stopped {
			return
		}
		if max > 0 && n >= max {
			r.Stop()
			return
		}
		if err := r.step(ctx); err != nil {
			r.mu.Lock()
			r.lastErr = err.Error()
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
}

func (r *Runtime) waitRunnable(ctx context.Context) error {
	for {
		r.mu.Lock()
		st := r.status
		r.mu.Unlock()
		switch st {
		case StatusRunning:
			return nil
		case StatusStopped:
			return fmt.Errorf("stopped")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stop:
			return fmt.Errorf("stopped")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (r *Runtime) step(ctx context.Context) error {
	totalStart := time.Now()
	preStart := time.Now()
	obs, err := r.game.Observe(ctx)
	pre := time.Since(preStart)
	if err != nil {
		return err
	}
	r.capturePreview()

	r.mu.Lock()
	hist := append([]game.Action(nil), r.history...)
	goal := r.cfg.Goal
	model := r.cfg.Model
	n := r.decision + 1
	subgoal := r.subgoal
	lastScene := r.lastScene
	lastOutcome := r.lastOutcome
	lastExpected := r.lastExpected
	r.mu.Unlock()

	if r.sess != nil && r.sess.ShouldSaveDecisionFrame() {
		if _, err := r.sess.SaveFrame(obs.Image); err != nil {
			return err
		}
	}

	dec, err := r.agent.Decide(ctx, agent.DecisionRequest{
		Observation:  obs,
		Game:         r.game.Name(),
		Goal:         goal,
		Actions:      r.game.Actions(),
		History:      hist,
		Subgoal:      subgoal,
		LastScene:    lastScene,
		LastOutcome:  lastOutcome,
		LastExpected: lastExpected,
	})
	if err != nil {
		return err
	}

	requestedRepeat := boundedRepeat(dec.Action, dec.Repeat)
	emuStart := time.Now()
	appliedRepeat, outcome, changeScore, err := r.applyVerified(ctx, obs, dec.Action, requestedRepeat)
	emu := time.Since(emuStart)
	if err != nil {
		return err
	}
	total := time.Since(totalStart)
	r.capturePreview()

	r.mu.Lock()
	r.decision = n
	r.lastAct = dec.Action.Name
	r.lastSrc = string(game.SourceAgent)
	r.lastLat = total
	r.lastRepeat = appliedRepeat
	r.lastOutcome = outcome
	r.lastChange = changeScore
	r.lastExpected = dec.Expected
	if dec.Subgoal != "" {
		r.subgoal = dec.Subgoal
	}
	if dec.Scene != "" {
		r.lastScene = dec.Scene
	}
	for i := 0; i < appliedRepeat; i++ {
		r.pushHistory(dec.Action)
	}
	if dec.Invalid {
		r.invalid++
	}
	if dec.Timeout {
		r.timeouts++
	}
	if outcome == string(vision.ChangeNone) {
		r.repeated++
	}
	if dec.Error != "" || dec.Timeout || dec.Invalid {
		r.consecErr++
	} else {
		r.consecErr = 0
	}
	stopForErrors := r.cfg.MaxConsecutiveErrors > 0 && r.consecErr >= r.cfg.MaxConsecutiveErrors
	r.tracker.Add(metrics.Sample{
		Preprocess: pre,
		Inference:  dec.Inference,
		Parse:      dec.Parse,
		Emulator:   emu,
		Total:      total,
		Invalid:    dec.Invalid,
		Timeout:    dec.Timeout,
	})
	persistedSubgoal := r.subgoal
	r.mu.Unlock()

	if r.sess != nil {
		_ = r.sess.Log(recording.Event{
			DecisionNumber:    n,
			Timestamp:         time.Now(),
			Source:            string(game.SourceAgent),
			Model:             model,
			FrameSize:         fmt.Sprintf("%dx%d", obs.Width, obs.Height),
			Frame:             obs.Frame,
			PreprocessNs:      pre.Nanoseconds(),
			InferenceNs:       dec.Inference.Nanoseconds(),
			ParseNs:           dec.Parse.Nanoseconds(),
			EmulatorNs:        emu.Nanoseconds(),
			TotalNs:           total.Nanoseconds(),
			RawResponse:       dec.Raw,
			ParsedAction:      dec.Action.Name,
			Scene:             dec.Scene,
			Subgoal:           persistedSubgoal,
			Expected:          dec.Expected,
			Confidence:        dec.Confidence,
			RequestedRepeat:   requestedRepeat,
			AppliedRepeat:     appliedRepeat,
			VisualOutcome:     outcome,
			VisualChangeScore: changeScore,
			Invalid:           dec.Invalid,
			Timeout:           dec.Timeout,
			Retried:           dec.Retried,
			Error:             dec.Error,
			RepeatedState:     outcome == string(vision.ChangeNone),
		})
	}
	if stopForErrors {
		r.mu.Lock()
		r.lastErr = "too many consecutive model errors"
		r.mu.Unlock()
		r.Stop()
	}
	return nil
}

// applyVerified executes a bounded controller burst and visually verifies every
// individual input. A burst continues only while the frame shows ordinary
// visual progress. It stops immediately on no change, a major scene change, or
// an unclassifiable frame so the model can look again before doing more damage.
func (r *Runtime) applyVerified(ctx context.Context, before game.Observation, action game.Action, repeat int) (int, string, float64, error) {
	current := before
	outcome := string(vision.ChangeUnknown)
	changeScore := 0.0
	applied := 0
	for i := 0; i < repeat; i++ {
		if err := r.game.Apply(ctx, action); err != nil {
			return applied, outcome, changeScore, err
		}
		applied++
		after, err := r.game.Observe(ctx)
		if err != nil {
			return applied, outcome, changeScore, err
		}
		delta, err := vision.ComparePNG(current.Image, after.Image)
		if err != nil {
			outcome = string(vision.ChangeUnknown)
			break
		}
		outcome = string(delta.Kind)
		changeScore = delta.ChangedFraction
		current = after
		if delta.Kind != vision.ChangeSome {
			break
		}
	}
	return applied, outcome, changeScore, nil
}

func boundedRepeat(action game.Action, requested int) int {
	if requested <= 0 {
		requested = 1
	}
	if requested > 4 {
		requested = 4
	}
	switch strings.ToUpper(strings.TrimSpace(action.Name)) {
	case "UP", "DOWN", "LEFT", "RIGHT":
		return requested
	default:
		return 1
	}
}

func (r *Runtime) pushHistory(a game.Action) {
	r.history = append(r.history, a)
	if len(r.history) > r.cfg.History {
		r.history = r.history[len(r.history)-r.cfg.History:]
	}
}

func (r *Runtime) capturePreview() {
	p, ok := r.game.(game.Previewable)
	if !ok {
		return
	}
	png, err := p.PreviewPNG()
	if err != nil {
		return
	}
	r.mu.Lock()
	r.preview = png
	r.mu.Unlock()
}

func (r *Runtime) Human(ctx context.Context, action game.Action) error {
	before, _ := r.game.Observe(ctx)
	emuStart := time.Now()
	if err := r.game.Apply(ctx, action); err != nil {
		return err
	}
	emu := time.Since(emuStart)
	after, _ := r.game.Observe(ctx)
	outcome := string(vision.ChangeUnknown)
	changeScore := 0.0
	if len(before.Image) != 0 && len(after.Image) != 0 {
		if delta, err := vision.ComparePNG(before.Image, after.Image); err == nil {
			outcome = string(delta.Kind)
			changeScore = delta.ChangedFraction
		}
	}
	r.capturePreview()
	r.mu.Lock()
	r.lastAct = action.Name
	r.lastSrc = string(game.SourceHuman)
	r.lastOutcome = outcome
	r.lastChange = changeScore
	r.lastRepeat = 1
	r.pushHistory(action)
	r.mu.Unlock()
	if r.sess != nil {
		_ = r.sess.Log(recording.Event{
			Timestamp:         time.Now(),
			Source:            string(game.SourceHuman),
			ParsedAction:      action.Name,
			EmulatorNs:        emu.Nanoseconds(),
			AppliedRepeat:     1,
			VisualOutcome:     outcome,
			VisualChangeScore: changeScore,
			RepeatedState:     outcome == string(vision.ChangeNone),
		})
	}
	return nil
}

func (r *Runtime) Pause() {
	r.mu.Lock()
	if r.status == StatusRunning {
		r.status = StatusPaused
	}
	r.mu.Unlock()
}

func (r *Runtime) Resume() {
	r.mu.Lock()
	if r.status == StatusPaused {
		r.status = StatusRunning
	}
	r.mu.Unlock()
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.status = StatusStopped
	r.stopped = true
	close(r.stop)
	r.mu.Unlock()
}

func (r *Runtime) Wait() {
	<-r.stop
}

func (r *Runtime) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := time.Duration(0)
	if !r.started.IsZero() {
		run = time.Since(r.started)
	}
	recent := make([]string, len(r.history))
	for i, a := range r.history {
		recent[i] = a.Name
	}
	return Snapshot{
		Status:          r.status,
		Game:            r.game.Name(),
		Goal:            r.cfg.Goal,
		Subgoal:         r.subgoal,
		Model:           r.cfg.Model,
		Decision:        r.decision,
		LastAction:      r.lastAct,
		LastSource:      r.lastSrc,
		LastScene:       r.lastScene,
		LastOutcome:     r.lastOutcome,
		LastExpected:    r.lastExpected,
		LastRepeat:      r.lastRepeat,
		LastChangeScore: r.lastChange,
		Recent:          recent,
		LastLatency:     r.lastLat,
		LastLatencyMS:   float64(r.lastLat) / float64(time.Millisecond),
		Runtime:         run,
		RuntimeHuman:    run.Truncate(time.Second).String(),
		Invalid:         r.invalid,
		Timeouts:        r.timeouts,
		RepeatedState:   r.repeated,
		Error:           r.lastErr,
	}
}

func (r *Runtime) PreviewPNG() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.preview
}

func (r *Runtime) Summary() metrics.Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := time.Duration(0)
	if !r.started.IsZero() {
		run = time.Since(r.started)
	}
	return r.tracker.Summarize(run)
}

func (r *Runtime) SaveState() ([]byte, error) {
	st, ok := r.game.(game.Stateful)
	if !ok {
		return nil, fmt.Errorf("runtime: game does not support save states")
	}
	return st.SaveState()
}

func (r *Runtime) SetModel(name string) {
	r.mu.Lock()
	r.cfg.Model = name
	r.mu.Unlock()
}
