package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maestroi/gamevision/internal/inference"
	"github.com/maestroi/gamevision/internal/runtime"
	"github.com/maestroi/gamevision/pkg/game"
)

// Server is an observational control plane. It is not the emulator clock.
type Server struct {
	rt  *runtime.Runtime
	inf *inference.Client
	dir string
}

func New(rt *runtime.Runtime, inf *inference.Client, sessionDir string) *Server {
	return &Server{rt: rt, inf: inf, dir: sessionDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.rt.Snapshot())
	})
	mux.HandleFunc("/frame.png", func(w http.ResponseWriter, r *http.Request) {
		png := s.rt.PreviewPNG()
		if png == nil {
			http.Error(w, "no frame yet", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/control", s.handleControl)
	mux.HandleFunc("/input", s.handleInput)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/models", s.handleModels)
	mux.HandleFunc("/model", s.handleModel)
	return mux
}

func (s *Server) Listen(addr string) (string, *http.Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	hs := &http.Server{Handler: s.Handler()}
	go func() { _ = hs.Serve(ln) }()
	return ln.Addr().String(), hs, nil
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch strings.ToLower(body.Cmd) {
	case "pause":
		s.rt.Pause()
	case "resume", "start":
		s.rt.Resume()
	case "stop":
		s.rt.Stop()
	default:
		http.Error(w, "unknown cmd", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.rt.Snapshot())
}

func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.ToUpper(strings.TrimSpace(body.Action))
	if name == "" {
		http.Error(w, "action required", http.StatusBadRequest)
		return
	}
	if err := s.rt.Human(r.Context(), game.Action{Name: name}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.rt.Snapshot())
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	data, err := s.rt.SaveState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := time.Now().Format("state-150405.state")
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": path})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if s.inf == nil {
		http.Error(w, "no inference client", http.StatusServiceUnavailable)
		return
	}
	models, err := s.inf.ListModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"current": s.inf.Model(),
		"models":  models,
	})
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.inf == nil {
		http.Error(w, "no inference client", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wasRunning := s.rt.Snapshot().Status == runtime.StatusRunning
	s.rt.Pause()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	info, err := s.inf.Switch(ctx, body.Model)
	if err != nil {
		if wasRunning {
			s.rt.Resume()
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.rt.SetModel(info.ID)
	if wasRunning {
		s.rt.Resume()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"loaded":   info,
		"snapshot": s.rt.Snapshot(),
	})
}
