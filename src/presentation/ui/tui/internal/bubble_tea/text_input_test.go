package bubble_tea

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTextInput(t *testing.T) {
	placeholder := "Enter text"
	ti := NewTextInput(placeholder)
	if ti.ti.Placeholder != placeholder {
		t.Errorf("Expected placeholder %q, got %q", placeholder, ti.ti.Placeholder)
	}
	if !ti.ti.Focused() {
		t.Error("Expected input to be focused")
	}
	if ti.ti.CharLimit != 256 {
		t.Errorf("Expected CharLimit 256, got %d", ti.ti.CharLimit)
	}
	if ti.ti.Width != 40 {
		t.Errorf("Expected Width 40, got %d", ti.ti.Width)
	}
}

func TestValue(t *testing.T) {
	ti := NewTextInput("Test")
	ti.ti.SetValue("Hello")
	if got := ti.Value(); got != "Hello" {
		t.Errorf("Expected %q, got %q", "Hello", got)
	}
}

func TestInit(t *testing.T) {
	ti := NewTextInput("Test")
	cmd := ti.Init()
	if cmd == nil {
		t.Error("Expected non-nil command from Init()")
	}
}

func TestUpdateNonEnter(t *testing.T) {
	ti := NewTextInput("Test")
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	model, cmd := ti.Update(msg)
	if model == nil {
		t.Error("Expected non-nil model")
	}
	if cmd == nil {
		t.Error("Expected non-nil command on non-enter key")
	}
	done := make(chan tea.Msg, 1)
	go func() {
		done <- cmd()
	}()
	if msg := <-done; msg == nil {
		t.Error("Non-enter key command should not produce quit")
	}
}

func TestUpdateEnter(t *testing.T) {
	ti := NewTextInput("Test")
	msg := tea.KeyMsg{Runes: []rune("enter")}
	model, cmd := ti.Update(msg)
	if model != ti {
		t.Error("Expected model to remain unchanged on enter key")
	}
	if cmd == nil {
		t.Error("Expected non-nil command from enter key")
	}
	quitMsg := cmd()
	// Check that the returned message is of type cursor.BlinkMsg.
	typeName := fmt.Sprintf("%T", quitMsg)
	expectedType := "cursor.BlinkMsg"
	if typeName != expectedType {
		t.Errorf("Expected tea.Quit command to return %s, got %s", expectedType, typeName)
	}
}

func TestUpdateEnter_KeyTypeEnter_Quits(t *testing.T) {
	ti := NewTextInput("Test")
	msg := tea.KeyMsg{Type: tea.KeyEnter}

	model, cmd := ti.Update(msg)
	if model != ti {
		t.Fatal("expected model to remain unchanged")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command from enter key")
	}
	got := cmd()
	if _, ok := got.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", got)
	}
}

func TestUpdateEsc_KeyTypeEsc_CancelsAndQuits(t *testing.T) {
	ti := NewTextInput("Test")
	msg := tea.KeyMsg{Type: tea.KeyEsc}

	model, cmd := ti.Update(msg)
	if model != ti {
		t.Fatal("expected model to remain unchanged")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command from esc key")
	}
	if !ti.Cancelled() {
		t.Fatal("expected cancelled state after esc")
	}
	got := cmd()
	if _, ok := got.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", got)
	}
}

func TestView(t *testing.T) {
	ti := NewTextInput("Test")
	view := ti.View()
	if view == "" {
		t.Error("Expected non-empty view output")
	}
}

func TestInputContainerWidth_FallbackToTextInputWidth(t *testing.T) {
	ti := NewTextInput("Test")
	ti.width = 0
	ti.ti.Width = 19

	got := ti.inputContainerWidth()
	want := maxInt(1, ti.ti.Width+resolveUIStyles(ti.settings.Preferences()).inputFrame.GetHorizontalFrameSize())
	if got != want {
		t.Fatalf("expected fallback width %d, got %d", want, got)
	}
}

func TestInputContainerWidth_UsesTerminalWidthWhenKnown(t *testing.T) {
	ti := NewTextInput("Test")
	ti.width = 120

	got := ti.inputContainerWidth()
	want := maxInt(1, contentWidthForTerminal(120))
	if got != want {
		t.Fatalf("expected width from terminal content %d, got %d", want, got)
	}
}

func TestUpdateWindowSize_ClampsToCardContentWidth(t *testing.T) {
	ti := NewTextInput("Test")
	_, _ = ti.Update(tea.WindowSizeMsg{Width: 220, Height: 40})

	maxAllowed := contentWidthForTerminal(220) - resolveUIStyles(ti.settings.Preferences()).inputFrame.GetHorizontalFrameSize()
	if ti.ti.Width > maxAllowed {
		t.Fatalf("expected width <= %d, got %d", maxAllowed, ti.ti.Width)
	}
	if ti.ti.Width > 40 {
		t.Fatalf("expected width to stay stable and not exceed 40, got %d", ti.ti.Width)
	}
	if ti.ti.Width < 1 {
		t.Fatalf("expected positive width, got %d", ti.ti.Width)
	}
}
