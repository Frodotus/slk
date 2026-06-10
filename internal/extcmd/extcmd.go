// Package extcmd runs user-configured external commands against a
// selected Slack message. Commands are executed directly (no shell, so no
// quoting/injection hazard): the message text is piped to stdin and
// metadata is exposed via SLK_* environment variables.
package extcmd

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gammons/slk/internal/config"
)

// Context is the message data handed to a command.
type Context struct {
	Text        string
	UserName    string
	UserID      string
	TS          string
	ChannelID   string
	ChannelName string
	Permalink   string
	ImagePaths  []string // on-disk paths of the message's cached images
}

// Result captures a finished non-interactive run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error // non-nil if the command failed to start or exited non-zero
}

// env builds the process environment: the inherited environment plus the
// SLK_* variables describing the message.
func env(ctx Context) []string {
	e := os.Environ()
	return append(e,
		"SLK_TEXT="+ctx.Text,
		"SLK_USER="+ctx.UserName,
		"SLK_USER_ID="+ctx.UserID,
		"SLK_TS="+ctx.TS,
		"SLK_CHANNEL="+ctx.ChannelID,
		"SLK_CHANNEL_NAME="+ctx.ChannelName,
		"SLK_PERMALINK="+ctx.Permalink,
		"SLK_IMAGE_PATHS="+strings.Join(ctx.ImagePaths, "\n"),
		"SLK_IMAGE_COUNT="+strconv.Itoa(len(ctx.ImagePaths)),
	)
}

// BuildCmd constructs the *exec.Cmd for a command + message context with
// argv and environment set, but leaves stdio to the caller. Used by both
// the captured runner (Run) and the interactive path (which hands the
// *exec.Cmd to bubbletea's ExecProcess so it inherits the terminal).
// Callers must check len(c.Argv) > 0 (config.Load guarantees this for
// loaded commands).
func BuildCmd(c config.ExternalCommand, ctx Context) *exec.Cmd {
	cmd := exec.Command(c.Argv[0], c.Argv[1:]...)
	cmd.Env = env(ctx)
	return cmd
}

// Run executes a command non-interactively: the message text is piped to
// stdin and stdout/stderr are captured. Blocking — callers run it in a
// goroutine.
func Run(c config.ExternalCommand, ctx Context) Result {
	cmd := BuildCmd(c, ctx)
	cmd.Stdin = strings.NewReader(ctx.Text)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errBuf.String(), Err: err}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
	}
	return res
}

// FirstLine returns the first non-empty line of s, trimmed — handy for a
// terse toast from a command's stderr.
func FirstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}
