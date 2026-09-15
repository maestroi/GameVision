package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvesRelativeState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bedroom.json")
	if err := os.WriteFile(path, []byte(`{"name":"bedroom","goal":"Leave.","state":"bedroom.state","max_decisions":9}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if sc.MaxDecisions != 9 || sc.Goal != "Leave." {
		t.Fatalf("%+v", sc)
	}
	if sc.State != filepath.Join(dir, "bedroom.state") {
		t.Fatalf("state %q", sc.State)
	}
}
