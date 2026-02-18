package bubble_tea

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTextArea(t *testing.T) {
	ta := NewTextArea("Type here...")
	if ta == nil {
		t.Fatal("Expected non-nil TextArea")
	}
	if ta.ta.Placeholder != "Type here..." {
		t.Errorf("Expected placeholder %q, got %q", "Type here...", ta.ta.Placeholder)
	}
}

func TestTextArea_Value(t *testing.T) {
	ta := NewTextArea("Type here...")
	if ta.Value() != "" {
		t.Errorf("Expected initial value to be empty, got %q", ta.Value())
	}
}

func TestTextArea_Init(t *testing.T) {
	ta := NewTextArea("Type here...")
	cmd := ta.Init()
	if cmd == nil {
		t.Error("Expected non-nil command from Init")
	}
}

func TestTextArea_UpdateEnter(t *testing.T) {
	ta := NewTextArea("Type here...")
	msg := tea.KeyMsg{Type: tea.KeyEnter, Runes: []rune("enter")}
	model, cmd := ta.Update(msg)

	// Ensure that model is of the correct type
	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	// Check that the command is non-nil (tea.Quit is returned on "enter")
	if cmd == nil {
		t.Error("Expected a non-nil quit command on enter")
	}
	// The underlying value should remain unchanged if no text was added.
	if updatedTA.Value() != "" {
		t.Errorf("Expected value to remain empty, got %q", updatedTA.Value())
	}
}

func TestTextArea_UpdateEsc_CancelsAndQuits(t *testing.T) {
	ta := NewTextArea("Type here...")
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	model, cmd := ta.Update(msg)

	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	if cmd == nil {
		t.Error("Expected a non-nil quit command on esc")
	}
	if !updatedTA.Cancelled() {
		t.Fatal("expected cancelled state after esc")
	}
}

func TestTextArea_UpdateOther(t *testing.T) {
	ta := NewTextArea("Type here...")
	initialValue := ta.Value()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	model, cmd := ta.Update(msg)
	_ = cmd // command isn't checked here

	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	updatedValue := updatedTA.Value()
	if initialValue == updatedValue {
		t.Error("Expected the value to update after key press")
	}
}

func TestTextArea_View(t *testing.T) {
	ta := NewTextArea("Type here...")
	view := ta.View()
	if !strings.Contains(view, ta.ta.Placeholder) {
		t.Errorf("Expected view to contain placeholder %q, got %q", ta.ta.Placeholder, view)
	}
}

func TestTextArea_View_WhenDone_Empty(t *testing.T) {
	ta := NewTextArea("Type here...")
	_, _ = ta.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if view := ta.View(); view != "" {
		t.Fatalf("expected empty view when done, got %q", view)
	}
}

func TestTextArea_UpdateWindowSize_ClampsToCardContentWidth(t *testing.T) {
	ta := NewTextArea("Type here...")
	_, _ = ta.Update(tea.WindowSizeMsg{Width: 220, Height: 40})

	maxAllowed := contentWidthForTerminal(220) - inputContainerStyle().GetHorizontalFrameSize()
	if ta.ta.Width() > maxAllowed {
		t.Fatalf("expected width <= %d, got %d", maxAllowed, ta.ta.Width())
	}
	if ta.ta.Width() > 80 {
		t.Fatalf("expected width to stay stable and not exceed 80, got %d", ta.ta.Width())
	}
	if ta.ta.Width() < 1 {
		t.Fatalf("expected positive width, got %d", ta.ta.Width())
	}
}
