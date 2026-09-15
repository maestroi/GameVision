package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Spec is a reproducible starting environment. The save state is not shown to
// the model; it only establishes where the experiment begins.
type Spec struct {
	Name         string `json:"name"`
	Game         string `json:"game"`
	Goal         string `json:"goal"`
	State        string `json:"state"`
	MaxDecisions int    `json:"max_decisions"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return Spec{}, fmt.Errorf("scenario: %w", err)
	}
	if s.Name == "" {
		s.Name = filepath.Base(path)
	}
	if s.State != "" && !filepath.IsAbs(s.State) {
		s.State = filepath.Join(filepath.Dir(path), s.State)
	}
	return s, nil
}
