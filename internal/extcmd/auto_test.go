package extcmd

import (
	"testing"

	"github.com/gammons/slk/internal/config"
)

// TestAutoTrigger is the headline of auto-execution: CompileAuto selects
// the commands with triggers (compiling regexes), and Triggered fires on
// mention / substring / regex, honoring the channel filter.
func TestAutoTrigger(t *testing.T) {
	cmds := []config.ExternalCommand{
		{Name: "mention", Argv: []string{"x"}, OnMention: true},
		{Name: "sub", Argv: []string{"x"}, Match: "Deploy Failed"},
		{Name: "re", Argv: []string{"x"}, MatchRegex: `error: \d+`},
		{Name: "scoped", Argv: []string{"x"}, Match: "alert", Channels: []string{"ops"}},
		{Name: "manual", Argv: []string{"x"}},                 // no trigger → excluded
		{Name: "badre", Argv: []string{"x"}, MatchRegex: "("}, // invalid regex
	}
	auto, errs := CompileAuto(cmds)
	if len(auto) != 5 {
		t.Fatalf("expected 5 auto commands (manual excluded), got %d", len(auto))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 regex compile error, got %d", len(errs))
	}

	byName := map[string]AutoCommand{}
	for _, a := range auto {
		byName[a.Cmd.Name] = a
	}

	if !byName["mention"].Triggered("hey <@U1> look", true, "general") {
		t.Error("mention: should fire when mentioned")
	}
	if byName["mention"].Triggered("hey", false, "general") {
		t.Error("mention: should not fire without a mention")
	}
	if !byName["sub"].Triggered("the DEPLOY FAILED again", false, "x") {
		t.Error("substring: should match case-insensitively")
	}
	if byName["sub"].Triggered("all green", false, "x") {
		t.Error("substring: should not match unrelated text")
	}
	if !byName["re"].Triggered("got error: 42 now", false, "x") {
		t.Error("regex: should match")
	}
	if byName["re"].Triggered("error: none", false, "x") {
		t.Error("regex: should not match")
	}
	if !byName["scoped"].Triggered("alert!", false, "ops") {
		t.Error("channel filter: should fire in an allowed channel")
	}
	if byName["scoped"].Triggered("alert!", false, "general") {
		t.Error("channel filter: should not fire outside allowed channels")
	}
	// Invalid regex didn't compile and there's no other trigger → never fires.
	if byName["badre"].Triggered("(", false, "x") {
		t.Error("invalid regex must not fire")
	}
}
