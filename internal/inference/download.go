package inference

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type hfBundle struct {
	Repo   string
	Model  string
	MMProj string
}

func hfFileURL(repo, file string) string {
	return "https://huggingface.co/" + repo + "/resolve/main/" + file
}

func bundleDir(repo string) string {
	rel := filepath.FromSlash(repo)
	if d := os.Getenv("GAMEVISION_MODELS_DIR"); d != "" {
		return filepath.Join(d, rel)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return rel
	}
	return filepath.Join(home, ".lmstudio", "models", rel)
}

func ensureLocalModel(ctx context.Context, spec *launchSpec) error {
	if spec.GGUF != "" && spec.MMProj != "" {
		return nil
	}
	if spec.HFRepo == "" || spec.HFFile == "" {
		return fmt.Errorf("inference: no local GGUF and no Hugging Face files for %s", spec.Alias)
	}
	dir := bundleDir(spec.HFRepo)
	if spec.GGUF == "" {
		spec.GGUF = filepath.Join(dir, spec.HFFile)
	}
	if spec.MMProj == "" && spec.HFMMProj != "" {
		spec.MMProj = filepath.Join(dir, spec.HFMMProj)
	}
	if err := downloadHTTP(ctx, hfFileURL(spec.HFRepo, spec.HFFile), spec.GGUF); err != nil {
		return err
	}
	if spec.MMProj != "" {
		if err := downloadHTTP(ctx, hfFileURL(spec.HFRepo, spec.HFMMProj), spec.MMProj); err != nil {
			return err
		}
	}
	return nil
}

func downloadHTTP(ctx context.Context, fileURL, dest string) error {
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		logf("already have %s (%s)", dest, byteSize(st.Size()))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	logf("downloading %s", fileURL)
	logf("         -> %s", dest)
	curl, err := exec.LookPath("curl")
	if err != nil {
		return fmt.Errorf("inference: curl is required to download GGUFs (this llama-server build cannot use -hf: no HTTPS)")
	}
	args := []string{
		"-L", "--fail", "--retry", "5",
		"--continue-at", "-", "--progress-bar",
		"-o", tmp, fileURL,
	}
	if tok := firstEnv("HF_TOKEN", "HUGGING_FACE_HUB_TOKEN"); tok != "" {
		args = append([]string{"-H", "Authorization: Bearer " + tok}, args...)
	}
	cmd := exec.CommandContext(ctx, curl, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("inference: download %s: %w", fileURL, err)
	}
	st, err := os.Stat(tmp)
	if err != nil || st.Size() == 0 {
		return fmt.Errorf("inference: download produced an empty file: %s", tmp)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	logf("downloaded %s (%s)", dest, byteSize(st.Size()))
	return nil
}

func byteSize(n int64) string {
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	return fmt.Sprintf("%d bytes", n)
}

func logf(format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintln(os.Stderr, "inference: "+msg)
}
