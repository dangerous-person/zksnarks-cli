package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/ui"
)

func TestOldStateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := filepath.Join(dir, "zksnarks", "state.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	old := "{\n  \"query\": \"skele rim\",\n  \"grid\": true,\n  \"theme\": \"zec\",\n  \"section\": \"zilkroad\"\n}\n"
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	st := loadState()
	if st != (ui.State{Theme: "zec"}) {
		t.Fatalf("loaded %+v", st)
	}
	saveState(st)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"theme\": \"zec\"\n}\n" {
		t.Errorf("saved %q", data)
	}
}
