package extcmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gammons/slk/internal/config"
)

// AutoCommand is a config.ExternalCommand that auto-runs on matching
// messages, with its MatchRegex precompiled.
type AutoCommand struct {
	Cmd config.ExternalCommand
	re  *regexp.Regexp
}

// CompileAuto returns the subset of commands that have an auto-trigger,
// with their regexes compiled. A command with an invalid MatchRegex is
// still kept (its other triggers continue to work); the compile error is
// returned so the caller can log it.
func CompileAuto(cmds []config.ExternalCommand) ([]AutoCommand, []error) {
	var out []AutoCommand
	var errs []error
	for _, c := range cmds {
		if !c.HasAutoTrigger() {
			continue
		}
		ac := AutoCommand{Cmd: c}
		if c.MatchRegex != "" {
			re, err := regexp.Compile(c.MatchRegex)
			if err != nil {
				errs = append(errs, fmt.Errorf("command %q: invalid match_regex %q: %w", c.Name, c.MatchRegex, err))
			} else {
				ac.re = re
			}
		}
		out = append(out, ac)
	}
	return out, errs
}

// Triggered reports whether the command should auto-run for a message with
// the given text, mention status, and channel name. A channel filter (if
// set) gates all triggers; otherwise the command fires if any trigger
// matches.
func (a AutoCommand) Triggered(text string, mentioned bool, channelName string) bool {
	c := a.Cmd
	if len(c.Channels) > 0 && !channelAllowed(c.Channels, channelName) {
		return false
	}
	if c.OnMention && mentioned {
		return true
	}
	if c.Match != "" && strings.Contains(strings.ToLower(text), strings.ToLower(c.Match)) {
		return true
	}
	if a.re != nil && a.re.MatchString(text) {
		return true
	}
	return false
}

// channelAllowed reports whether name is in allowed (case-insensitive,
// leading '#' ignored on either side).
func channelAllowed(allowed []string, name string) bool {
	name = strings.TrimPrefix(name, "#")
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimPrefix(a, "#"), name) {
			return true
		}
	}
	return false
}
