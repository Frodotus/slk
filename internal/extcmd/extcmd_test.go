package extcmd

import (
	"testing"

	"github.com/gammons/slk/internal/config"
)

// TestRunPassesTextAndEnv is the headline of the engine: the message text
// reaches the command on stdin and the metadata reaches it via SLK_* env
// vars, with argv run directly (no shell on slk's side).
func TestRunPassesTextAndEnv(t *testing.T) {
	c := config.ExternalCommand{
		Name: "echo",
		Argv: []string{"sh", "-c", `printf '%s|%s|%s' "$SLK_TEXT" "$SLK_CHANNEL" "$SLK_IMAGE_COUNT"; cat`},
	}
	res := Run(c, Context{
		Text:       "hello world",
		ChannelID:  "C1",
		ImagePaths: []string{"/tmp/a.png", "/tmp/b.png"},
	})
	if res.Err != nil {
		t.Fatalf("run failed: %v (stderr=%q)", res.Err, res.Stderr)
	}
	// printf of env vars, then cat echoes stdin (the message text).
	want := "hello world|C1|2hello world"
	if res.Stdout != want {
		t.Errorf("stdout = %q, want %q", res.Stdout, want)
	}
}

func TestRunCapturesExitAndStderr(t *testing.T) {
	c := config.ExternalCommand{Name: "fail", Argv: []string{"sh", "-c", "echo oops 1>&2; exit 3"}}
	res := Run(c, Context{})
	if res.Err == nil {
		t.Error("expected non-nil Err on non-zero exit")
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if FirstLine(res.Stderr) != "oops" {
		t.Errorf("stderr first line = %q, want %q", FirstLine(res.Stderr), "oops")
	}
}
