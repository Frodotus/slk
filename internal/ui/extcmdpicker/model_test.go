package extcmdpicker

import "testing"

// TestPickerFilterSelect is the headline of the picker: typing filters the
// command list and Enter returns the original index of the highlighted
// command (so the caller maps it back to its config entry).
func TestPickerFilterSelect(t *testing.T) {
	m := New()
	m.SetItems([]string{"Create task", "OCR image", "Edit in nvim"})
	m.Open()
	if !m.IsVisible() {
		t.Fatal("picker should be visible after Open")
	}

	// Filter down to "OCR image" (index 1).
	m.HandleKey("o")
	m.HandleKey("c")
	m.HandleKey("r")

	res := m.HandleKey("enter")
	if res == nil {
		t.Fatal("expected a Result on enter")
	}
	if res.Index != 1 {
		t.Errorf("selected index = %d, want 1 (OCR image)", res.Index)
	}
	if m.IsVisible() {
		t.Error("picker should close after selecting")
	}
}

func TestPickerEscClosesWithoutResult(t *testing.T) {
	m := New()
	m.SetItems([]string{"a", "b"})
	m.Open()
	if res := m.HandleKey("esc"); res != nil {
		t.Errorf("esc must not return a result, got %+v", res)
	}
	if m.IsVisible() {
		t.Error("esc should close the picker")
	}
}
