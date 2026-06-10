package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExternalCommandsLoad checks that valid external commands load and
// malformed ones are dropped, and that interactive overrides capture.
func TestExternalCommandsLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	err := os.WriteFile(path, []byte(`
[[external_commands]]
name = "Create task"
argv = ["/bin/slk-task"]

[[external_commands]]
name = "OCR"
argv = ["tesseract", "-", "-"]
capture_output = true

[[external_commands]]
name = "Edit"
argv = ["nvim"]
interactive = true
capture_output = true

[[external_commands]]
name = "no argv"

[[external_commands]]
argv = ["x"]
`), 0644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Two malformed entries (missing argv, missing name) are dropped.
	if got := len(cfg.ExternalCommands); got != 3 {
		t.Fatalf("expected 3 valid commands, got %d", got)
	}
	if cfg.ExternalCommands[0].Name != "Create task" {
		t.Errorf("cmd[0] name = %q", cfg.ExternalCommands[0].Name)
	}
	if !cfg.ExternalCommands[1].CaptureOutput {
		t.Errorf("OCR should keep capture_output")
	}
	// interactive wins over capture_output.
	edit := cfg.ExternalCommands[2]
	if !edit.Interactive || edit.CaptureOutput {
		t.Errorf("interactive command must clear capture_output: %+v", edit)
	}
}
