package recording

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maestroi/gamevision/internal/metrics"
)

func TestSessionWritesArtifacts(t *testing.T) {
	root := t.TempDir()
	s, err := Open(Options{
		Root:               root,
		Game:               "pokemon-red",
		Model:              "Qwen3-VL-2B-Instruct",
		Goal:               "go",
		SaveDecisionFrames: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Log(Event{DecisionNumber: 1, Timestamp: time.Now(), Source: "agent", ParsedAction: "A"}); err != nil {
		t.Fatal(err)
	}
	name, err := s.SaveFrame([]byte("png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(metrics.Summary{Decisions: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "session.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "decisions.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "frames", name)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir(), "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta SessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Summary.Decisions != 1 || meta.Model != "Qwen3-VL-2B-Instruct" {
		t.Fatalf("%+v", meta)
	}
}
