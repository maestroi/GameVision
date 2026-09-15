package inference

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHFBundleKnownAliases(t *testing.T) {
	b, ok := hfBundleFor("Qwen3-VL-4B-Instruct")
	if !ok || b.Repo != "Qwen/Qwen3-VL-4B-Instruct-GGUF" || b.Model != "Qwen3VL-4B-Instruct-Q4_K_M.gguf" {
		t.Fatalf("%+v %v", b, ok)
	}
	b, ok = hfBundleFor("Qwen3-VL-2B-Instruct")
	if !ok || b.Repo != "Qwen/Qwen3-VL-2B-Instruct-GGUF" {
		t.Fatalf("%+v %v", b, ok)
	}
	if _, ok := hfBundleFor("Qwen3-VL"); ok {
		t.Fatal("ambiguous Qwen3-VL should not pick 2B vs 4B")
	}
}

func TestRestoreUnit(t *testing.T) {
	u, ok := restoreUnit("qwen3.8-27b")
	if !ok || u != "qwen38-solo.service" {
		t.Fatalf("%s %v", u, ok)
	}
	if _, ok := restoreUnit("Qwen3-VL-4B-Instruct"); ok {
		t.Fatal("VLM should not restore the coding unit")
	}
}

func TestFindLocalGGUF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Qwen3VL-4B-Instruct-Q4_K_M.gguf"), []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mmproj-Qwen3VL-4B-Instruct-F16.gguf"), []byte("mm"), 0o644); err != nil {
		t.Fatal(err)
	}
	gguf, mmproj, ok := findLocalGGUF("Qwen3-VL-4B-Instruct", dir)
	if !ok || !strings.HasSuffix(gguf, "Qwen3VL-4B-Instruct-Q4_K_M.gguf") {
		t.Fatalf("gguf=%q ok=%v", gguf, ok)
	}
	if !strings.HasSuffix(mmproj, "mmproj-Qwen3VL-4B-Instruct-F16.gguf") {
		t.Fatalf("mmproj=%q", mmproj)
	}
}

func TestResolveLaunchUsesHFWhenNoLocalGGUF(t *testing.T) {
	if llamaBin() == "" {
		t.Skip("no llama-server on this machine")
	}
	spec, err := resolveLaunch("Qwen3-VL-4B-Instruct", 8002, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if spec.HFRepo != "Qwen/Qwen3-VL-4B-Instruct-GGUF" || spec.HFFile != "Qwen3VL-4B-Instruct-Q4_K_M.gguf" {
		t.Fatalf("%+v", spec)
	}
	if spec.Alias != "Qwen3-VL-4B-Instruct" {
		t.Fatalf("alias %q", spec.Alias)
	}
	if spec.Port != 8002 || spec.Host != "127.0.0.1" {
		t.Fatalf("%+v", spec)
	}
}

func TestMissingModelHelp(t *testing.T) {
	msg := missingModelHelp("nope")
	if !strings.Contains(msg, "no /models/load catalog") {
		t.Fatal(msg)
	}
}

func TestHFFileURL(t *testing.T) {
	got := hfFileURL("Qwen/Qwen3-VL-4B-Instruct-GGUF", "Qwen3VL-4B-Instruct-Q4_K_M.gguf")
	want := "https://huggingface.co/Qwen/Qwen3-VL-4B-Instruct-GGUF/resolve/main/Qwen3VL-4B-Instruct-Q4_K_M.gguf"
	if got != want {
		t.Fatalf("%s", got)
	}
}

func TestEnsureLocalModelUsesExistingFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GAMEVISION_MODELS_DIR", root)
	dir := filepath.Join(root, "Qwen", "Qwen3-VL-4B-Instruct-GGUF")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gguf := filepath.Join(dir, "Qwen3VL-4B-Instruct-Q4_K_M.gguf")
	mm := filepath.Join(dir, "mmproj-Qwen3VL-4B-Instruct-F16.gguf")
	if err := os.WriteFile(gguf, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mm, []byte("mm"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := launchSpec{
		Alias:    "Qwen3-VL-4B-Instruct",
		HFRepo:   "Qwen/Qwen3-VL-4B-Instruct-GGUF",
		HFFile:   "Qwen3VL-4B-Instruct-Q4_K_M.gguf",
		HFMMProj: "mmproj-Qwen3VL-4B-Instruct-F16.gguf",
	}
	if err := ensureLocalModel(context.Background(), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.GGUF != gguf || spec.MMProj != mm {
		t.Fatalf("%+v", spec)
	}
}

func TestWaitHTTPOKFailsFastWhenUnitFailed(t *testing.T) {
	orig := unitState
	unitState = func(string) string { return "failed" }
	defer func() { unitState = orig }()
	err := waitHTTPOK(context.Background(), "http://127.0.0.1:1/health", "gamevision-llm.service")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("%v", err)
	}
}
