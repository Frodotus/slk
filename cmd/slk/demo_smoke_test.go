package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/config"
)

func TestDemoSeedRendersWithoutPanic(t *testing.T) {
	app := seedDemoApp(config.Default()).app
	app.Update(tea.WindowSizeMsg{Width: 170, Height: 44})
	v := app.View()
	s := v.Content
	if strings.TrimSpace(s) == "" {
		t.Fatal("demo View() rendered empty")
	}
	for _, want := range []string{"Starred", "engineering", "Sam Rivera", "Alice"} {
		if !strings.Contains(s, want) {
			t.Errorf("demo render missing %q", want)
		}
	}
}
