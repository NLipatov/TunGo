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

func TestView(t *testing.T) {
	ti := NewTextInput("Test")
	view := ti.View()
	if view == "" {
		t.Error("Expected non-empty view output")
	}
}
