package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// hostSwap is the single-model llama-server path. Tests replace it.
var hostSwap = switchHost

type launchSpec struct {
	Alias    string
	GGUF     string
	MMProj   string
	HFRepo   string
	HFFile   string
	HFMMProj string
	Bin      string
	LibDir   string
	Port     int
	Host     string
}

// Official Qwen VL GGUFs. This LM Studio llama-server has no HTTPS, so we
// download with curl and pass --model/--mmproj instead of -hf.
var knownHF = map[string]hfBundle{
	"qwen3-vl-2b-instruct": {
		Repo: "Qwen/Qwen3-VL-2B-Instruct-GGUF", Model: "Qwen3VL-2B-Instruct-Q4_K_M.gguf",
		MMProj: "mmproj-Qwen3VL-2B-Instruct-F16.gguf",
	},
	"qwen3-vl-4b-instruct": {
		Repo: "Qwen/Qwen3-VL-4B-Instruct-GGUF", Model: "Qwen3VL-4B-Instruct-Q4_K_M.gguf",
		MMProj: "mmproj-Qwen3VL-4B-Instruct-F16.gguf",
	},
	"qwen3vl-2b-instruct": {
		Repo: "Qwen/Qwen3-VL-2B-Instruct-GGUF", Model: "Qwen3VL-2B-Instruct-Q4_K_M.gguf",
		MMProj: "mmproj-Qwen3VL-2B-Instruct-F16.gguf",
	},
	"qwen3vl-4b-instruct": {
		Repo: "Qwen/Qwen3-VL-4B-Instruct-GGUF", Model: "Qwen3VL-4B-Instruct-Q4_K_M.gguf",
		MMProj: "mmproj-Qwen3VL-4B-Instruct-F16.gguf",
	},
}

var systemdRestore = map[string]string{
	"qwen3.8-27b": "qwen38-solo.service",
	"qwen38-solo": "qwen38-solo.service",
	"qwen38":      "qwen38-solo.service",
}

func switchHost(ctx context.Context, c *Client, name string) (ModelInfo, error) {
	name = strings.TrimSpace(name)
	port := originPort(c.Origin())
	if unit, ok := restoreUnit(name); ok {
		logf("restoring %s", unit)
		if err := restoreCodingServer(ctx, unit, c.Origin()); err != nil {
			return ModelInfo{}, err
		}
		c.SetModel("qwen3.8-27b")
		return ModelInfo{ID: "qwen3.8-27b", Status: "loaded"}, nil
	}
	spec, err := resolveLaunch(name, port, originHost(c.Origin()))
	if err != nil {
		return ModelInfo{}, err
	}
	logf("this llama-server cannot -hf (no HTTPS); downloading GGUFs with curl, then restarting :%d", port)
	if err := ensureLocalModel(ctx, &spec); err != nil {
		return ModelInfo{}, err
	}
	if spec.GGUF == "" || spec.MMProj == "" {
		return ModelInfo{}, fmt.Errorf("inference: need a GGUF and mmproj for %s", spec.Alias)
	}
	logf("stopping occupant on :%d (this unloads qwen3.8-27b)", port)
	if err := stopOccupant(ctx, port); err != nil {
		return ModelInfo{}, err
	}
	logf("starting llama-server --model %s", spec.GGUF)
	if err := startLlama(ctx, spec); err != nil {
		return ModelInfo{}, err
	}
	if err := waitHTTPOK(ctx, strings.TrimRight(c.Origin(), "/")+"/health", "gamevision-llm.service"); err != nil {
		return ModelInfo{}, err
	}
	if props, err := c.Props(ctx); err == nil && props.Alias != "" && !strings.EqualFold(props.Alias, spec.Alias) {
		return ModelInfo{}, fmt.Errorf("inference: server came up as %q, wanted %q", props.Alias, spec.Alias)
	}
	c.SetModel(spec.Alias)
	return ModelInfo{ID: spec.Alias, Status: "loaded", Path: spec.GGUF}, nil
}

func restoreUnit(name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if u, ok := systemdRestore[key]; ok {
		return u, true
	}
	if strings.Contains(key, "qwen3.8") && strings.Contains(key, "27") {
		return "qwen38-solo.service", true
	}
	return "", false
}

func restoreCodingServer(ctx context.Context, unit, origin string) error {
	_ = exec.CommandContext(ctx, "systemctl", "--user", "stop", "gamevision-llm.service").Run()
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "start", unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inference: start %s: %v (%s)", unit, err, strings.TrimSpace(string(out)))
	}
	return waitHTTPOK(ctx, strings.TrimRight(origin, "/")+"/health", unit)
}

func resolveLaunch(name string, port int, host string) (launchSpec, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	spec := launchSpec{
		Alias:  name,
		Port:   port,
		Host:   host,
		Bin:    llamaBin(),
		LibDir: llamaLibDir(),
	}
	if spec.Bin == "" {
		return spec, fmt.Errorf("inference: llama-server binary not found (set GAMEVISION_LLAMA_SERVER)")
	}
	if gguf, mmproj, ok := findLocalGGUF(name); ok {
		spec.GGUF = gguf
		spec.MMProj = mmproj
	}
	if b, ok := hfBundleFor(name); ok {
		spec.HFRepo = b.Repo
		spec.HFFile = b.Model
		spec.HFMMProj = b.MMProj
		return spec, nil
	}
	if spec.GGUF != "" {
		return spec, nil
	}
	return spec, fmt.Errorf("%s", missingModelHelp(name))
}

func hfBundleFor(name string) (hfBundle, bool) {
	key := normalizeModelKey(name)
	if b, ok := knownHF[key]; ok {
		return b, true
	}
	seen := map[string]hfBundle{}
	for alias, b := range knownHF {
		if strings.Contains(key, alias) || strings.Contains(alias, key) {
			seen[b.Repo] = b
		}
	}
	if len(seen) != 1 {
		return hfBundle{}, false
	}
	for _, b := range seen {
		return b, true
	}
	return hfBundle{}, false
}

func normalizeModelKey(name string) string {
	s := strings.ToLower(name)
	s = strings.TrimSuffix(s, ".gguf")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func compactModelKey(name string) string {
	return strings.ReplaceAll(normalizeModelKey(name), "-", "")
}

func modelKeysMatch(a, b string) bool {
	na, nb := normalizeModelKey(a), normalizeModelKey(b)
	if na == "" || nb == "" {
		return false
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return true
	}
	ca, cb := compactModelKey(na), compactModelKey(nb)
	return strings.Contains(ca, cb) || strings.Contains(cb, ca)
}

func findLocalGGUF(name string, extraDirs ...string) (gguf, mmproj string, ok bool) {
	want := normalizeModelKey(name)
	var hits []string
	dirs := append(extraDirs, modelDirs()...)
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			base := info.Name()
			if !strings.HasSuffix(strings.ToLower(base), ".gguf") {
				return nil
			}
			low := strings.ToLower(base)
			if strings.HasPrefix(low, "mmproj") {
				return nil
			}
			if modelKeysMatch(base, want) {
				hits = append(hits, path)
			}
			return nil
		})
	}
	if len(hits) == 0 {
		return "", "", false
	}
	gguf = hits[0]
	mmproj = siblingMMProj(filepath.Dir(gguf))
	return gguf, mmproj, true
}

func siblingMMProj(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var found string
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if strings.HasPrefix(n, "mmproj") && strings.HasSuffix(n, ".gguf") {
			if strings.Contains(n, "f16") || found == "" {
				found = filepath.Join(dir, e.Name())
			}
		}
	}
	return found
}

func modelDirs() []string {
	var dirs []string
	if d := os.Getenv("GAMEVISION_MODELS_DIR"); d != "" {
		dirs = append(dirs, d)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".lmstudio", "models"))
		dirs = append(dirs, filepath.Join(home, ".cache", "llama.cpp"))
	}
	return dirs
}

func llamaBin() string {
	if b := os.Getenv("GAMEVISION_LLAMA_SERVER"); b != "" {
		return b
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	pattern := filepath.Join(home, ".lmstudio", "extensions", "backends", "llama.cpp-linux-*-nvidia-cuda*", "llama-server")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p
	}
	return ""
}

func llamaLibDir() string {
	if d := os.Getenv("GAMEVISION_LLAMA_LIBDIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Getenv("LD_LIBRARY_PATH")
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".lmstudio", "extensions", "backends", "vendor", "linux-llama-cuda*"))
	vendor := ""
	if len(matches) > 0 {
		vendor = matches[len(matches)-1]
	}
	if extra := os.Getenv("LD_LIBRARY_PATH"); extra != "" {
		if vendor != "" {
			return vendor + ":" + extra
		}
		return extra
	}
	return vendor
}

func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return "127.0.0.1"
	}
	host := u.Hostname()
	if host == "localhost" {
		return "127.0.0.1"
	}
	return host
}

func originPort(origin string) int {
	u, err := url.Parse(origin)
	if err != nil {
		return 8002
	}
	if u.Port() != "" {
		p, _ := strconv.Atoi(u.Port())
		if p > 0 {
			return p
		}
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

func stopOccupant(ctx context.Context, port int) error {
	units := []string{
		"gamevision-llm.service",
		"qwen38-solo.service",
		"qwen38.service",
		"qwen38-vision.service",
	}
	_ = exec.CommandContext(ctx, "systemctl", "--user", "stop", units[0], units[1], units[2], units[3]).Run()
	_ = exec.CommandContext(ctx, "systemctl", "--user", "reset-failed", "gamevision-llm.service").Run()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !portOpen(port) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	if portOpen(port) {
		return fmt.Errorf("inference: port %d is still in use after stopping llama-server units", port)
	}
	return nil
}

func portOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 150*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func startLlama(ctx context.Context, spec launchSpec) error {
	if spec.GGUF == "" {
		return fmt.Errorf("inference: llama-server --model path is empty")
	}
	args := []string{
		"--user",
		"--unit=gamevision-llm",
		"--property=Restart=no",
	}
	if spec.LibDir != "" {
		args = append(args, "--setenv=LD_LIBRARY_PATH="+spec.LibDir)
	}
	if tok := firstEnv("HF_TOKEN", "HUGGING_FACE_HUB_TOKEN"); tok != "" {
		args = append(args, "--setenv=HF_TOKEN="+tok)
	}
	args = append(args, "--", spec.Bin, "--model", spec.GGUF)
	if spec.MMProj != "" {
		args = append(args, "--mmproj", spec.MMProj)
	}
	args = append(args,
		"--alias", spec.Alias,
		"--host", spec.Host,
		"--port", strconv.Itoa(spec.Port),
		"--ctx-size", "8192",
		"--gpu-layers", "all",
		"--flash-attn", "on",
		"--parallel", "1",
		"--image-min-tokens", "1024",
	)
	cmd := exec.CommandContext(ctx, "systemd-run", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inference: systemd-run llama-server: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func llamaJournal() string {
	cmd := exec.Command("journalctl", "--user", "-u", "gamevision-llm.service", "-n", "40", "--no-pager")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func unitActiveState(unit string) string {
	cmd := exec.Command("systemctl", "--user", "show", "-p", "ActiveState", "--value", unit)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var unitState = unitActiveState

func waitHTTPOK(ctx context.Context, healthURL, unit string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(15 * time.Minute)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	started := time.Now()
	var last error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if unit != "" {
			st := unitState(unit)
			if st == "failed" {
				logs := llamaJournal()
				if logs == "" {
					logs = st
				}
				return fmt.Errorf("inference: %s failed while waiting for %s\n%s", unit, healthURL, logs)
			}
			if time.Since(started) > 2*time.Second && (st == "inactive" || st == "dead") {
				logs := llamaJournal()
				return fmt.Errorf("inference: %s is %s while waiting for %s\n%s", unit, st, healthURL, logs)
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("HTTP %s", resp.Status)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	if logs := llamaJournal(); logs != "" {
		return fmt.Errorf("inference: started llama-server but it never became healthy: %w\n%s", last, logs)
	}
	return fmt.Errorf("inference: started llama-server but it never became healthy: %w", last)
}

func missingModelHelp(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "inference: %q is not the currently loaded model, and this llama.cpp server has no /models/load catalog.\n", name)
	fmt.Fprintf(&b, "Port 8002 is single-model llama-server (today: qwen3.8-27b, text-only).\n")
	fmt.Fprintf(&b, "Switch will stop qwen38-solo and start llama-server with a VLM GGUF.\n")
	fmt.Fprintf(&b, "No local GGUF matched %q under ~/.lmstudio/models.\n", name)
	fmt.Fprintf(&b, "Known Hugging Face files (downloaded with curl; llama-server -hf has no HTTPS here):\n")
	fmt.Fprintf(&b, "  Qwen3-VL-2B-Instruct -> Qwen/Qwen3-VL-2B-Instruct-GGUF Qwen3VL-2B-Instruct-Q4_K_M.gguf\n")
	fmt.Fprintf(&b, "  Qwen3-VL-4B-Instruct -> Qwen/Qwen3-VL-4B-Instruct-GGUF Qwen3VL-4B-Instruct-Q4_K_M.gguf (~2.5GB + mmproj)\n")
	fmt.Fprintf(&b, "Place GGUFs in ~/.lmstudio/models or set GAMEVISION_MODELS_DIR.\n")
	fmt.Fprintf(&b, "Restore the coding model with: gamevision models switch qwen3.8-27b")
	return b.String()
}

// Props is GET /props from llama-server.
type Props struct {
	Alias  string `json:"model_alias"`
	Path   string `json:"model_path"`
	Vision bool
}

func (c *Client) Props(ctx context.Context) (Props, error) {
	data, code, err := c.adminGET(ctx, c.Origin()+"/props")
	if err != nil {
		return Props{}, err
	}
	if code != 200 {
		return Props{}, fmt.Errorf("HTTP %d: %s", code, snippet(data))
	}
	var raw struct {
		Alias      string `json:"model_alias"`
		Path       string `json:"model_path"`
		Modalities struct {
			Vision bool `json:"vision"`
		} `json:"modalities"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Props{}, err
	}
	return Props{Alias: raw.Alias, Path: raw.Path, Vision: raw.Modalities.Vision}, nil
}
