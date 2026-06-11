package version

import (
	"strings"
	"testing"
)

func TestModalFooter(t *testing.T) {
	got := ModalFooter("v1.2.3")

	// Just the version.
	if got != "slk v1.2.3" {
		t.Errorf("footer = %q, want %q", got, "slk v1.2.3")
	}
}

func TestModalFooterUsesGivenVersion(t *testing.T) {
	got := ModalFooter("dev")
	if !strings.Contains(got, "slk dev") {
		t.Errorf("expected 'slk dev' in %q", got)
	}
}
