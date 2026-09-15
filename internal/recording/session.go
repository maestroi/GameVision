package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maestroi/gamevision/internal/metrics"
)

// Event is one JSONL line: an agent decision or a human input.
type Event struct {
	DecisionNumber int       `json:"decision_number,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	Source         string    `json:"source"`
	Model          string    `json:"model,omitempty"`
	FrameSize      string    `json:"frame_size,omitempty"`
	Frame          uint64    `json:"frame,omitempty"`
	PreprocessNs   int64     `json:"preprocessing_latency_ns,omitempty"`
	InferenceNs    int64     `json:"inference_latency_ns,omitempty"`
	ParseNs        int64     `json:"parse_latency_ns,omitempty"`
	EmulatorNs     int64     `json:"emulator_latency_ns,omitempty"`
	TotalNs        int64     `json:"total_decision_latency_ns,omitempty"`
	RawResponse    string    `json:"raw_response,omitempty"`
	ParsedAction   string    `json:"parsed_action"`
	Invalid        bool      `json:"invalid,omitempty"`
	Timeout        bool      `json:"timeout,omitempty"`
	Retried        bool      `json:"retried,omitempty"`
	Error          string    `json:"error,omitempty"`
	RepeatedState  bool      `json:"repeated_visual_state,omitempty"`
}

// SessionMeta is written to session.json at start and rewritten at stop.
type SessionMeta struct {
	Game               string          `json:"game"`
	Goal               string          `json:"goal"`
	Model              string          `json:"model"`
	BaseURL            string          `json:"base_url"`
	VisionScale        int             `json:"vision_scale"`
	HoldFrames         int             `json:"button_hold_frames"`
	SettleFrames       int             `json:"settle_frames"`
	History            int             `json:"history"`
	Started            time.Time       `json:"started"`
	Stopped            time.Time       `json:"stopped,omitempty"`
	Scenario           string          `json:"scenario,omitempty"`
	StatePath          string          `json:"state_path,omitempty"`
	SaveDecisionFrames bool            `json:"save_decision_frames"`
	SaveAllFrames      bool            `json:"save_all_frames"`
	Dir                string          `json:"dir"`
	Summary            metrics.Summary `json:"summary,omitempty"`
}

// Session writes artifacts under a directory.
type Session struct {
	mu      sync.Mutex
	dir     string
	frames  string
	meta    SessionMeta
	log     *os.File
	saveDec bool
	saveAll bool
	n       int
}

type Options struct {
	Root               string
	Game               string
	Goal               string
	Model              string
	BaseURL            string
	VisionScale        int
	HoldFrames         int
	SettleFrames       int
	History            int
	Scenario           string
	StatePath          string
	SaveDecisionFrames bool
	SaveAllFrames      bool
}

func Open(opt Options) (*Session, error) {
	root := opt.Root
	if root == "" {
		root = "sessions"
	}
	stamp := time.Now().Format("2006-01-02-150405")
	model := sanitize(opt.Model)
	if model == "" {
		model = "model"
	}
	gameName := sanitize(opt.Game)
	if gameName == "" {
		gameName = "game"
	}
	dir := filepath.Join(root, stamp+"-"+gameName+"-"+model)
	frames := filepath.Join(dir, "frames")
	if err := os.MkdirAll(frames, 0o755); err != nil {
		return nil, err
	}
	logf, err := os.Create(filepath.Join(dir, "decisions.jsonl"))
	if err != nil {
		return nil, err
	}
	s := &Session{
		dir:     dir,
		frames:  frames,
		saveDec: opt.SaveDecisionFrames,
		saveAll: opt.SaveAllFrames,
		meta: SessionMeta{
			Game:               opt.Game,
			Goal:               opt.Goal,
			Model:              opt.Model,
			BaseURL:            opt.BaseURL,
			VisionScale:        opt.VisionScale,
			HoldFrames:         opt.HoldFrames,
			SettleFrames:       opt.SettleFrames,
			History:            opt.History,
			Started:            time.Now(),
			Scenario:           opt.Scenario,
			StatePath:          opt.StatePath,
			SaveDecisionFrames: opt.SaveDecisionFrames,
			SaveAllFrames:      opt.SaveAllFrames,
			Dir:                dir,
		},
	}
	s.log = logf
	if err := s.writeMeta(); err != nil {
		_ = logf.Close()
		return nil, err
	}
	return s, nil
}

func (s *Session) Dir() string { return s.dir }

func (s *Session) Log(ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enc := json.NewEncoder(s.log)
	return enc.Encode(ev)
}

func (s *Session) SaveFrame(png []byte) (string, error) {
	if png == nil {
		return "", nil
	}
	s.mu.Lock()
	s.n++
	n := s.n
	s.mu.Unlock()
	name := fmt.Sprintf("%06d.png", n)
	path := filepath.Join(s.frames, name)
	return name, os.WriteFile(path, png, 0o644)
}

func (s *Session) ShouldSaveDecisionFrame() bool {
	return s.saveDec || s.saveAll
}

func (s *Session) ShouldSaveAllFrames() bool { return s.saveAll }

func (s *Session) Finish(sum metrics.Summary) error {
	s.mu.Lock()
	s.meta.Stopped = time.Now()
	s.meta.Summary = sum
	s.mu.Unlock()
	if err := s.writeMeta(); err != nil {
		return err
	}
	return s.log.Close()
}

func (s *Session) writeMeta() error {
	data, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "session.json"), append(data, '\n'), 0o644)
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-' || r == '_':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
