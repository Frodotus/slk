package emoji

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type urlFixture struct {
	Base    string            `json:"base"`
	Entries []urlFixtureEntry `json:"entries"`
}

type urlFixtureEntry struct {
	Name       string `json:"name"`
	Codepoints []rune `json:"codepoints"`
	URL        string `json:"url"`
}

func loadURLFixture(t *testing.T) urlFixture {
	t.Helper()
	path := filepath.Join("testdata", "slack_urls.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f urlFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(f.Entries) == 0 {
		t.Fatalf("fixture has no entries")
	}
	return f
}

func TestBuildStandardEmojiURL(t *testing.T) {
	fixture := loadURLFixture(t)
	for _, e := range fixture.Entries {
		got := BuildStandardEmojiURL(e.Codepoints)
		if got != e.URL {
			t.Errorf("BuildStandardEmojiURL(%q codepoints=%v) = %q, want %q",
				e.Name, e.Codepoints, got, e.URL)
		}
	}
}
