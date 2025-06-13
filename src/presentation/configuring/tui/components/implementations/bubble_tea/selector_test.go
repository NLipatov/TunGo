package bubble_tea

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewSelector(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	if sel.placeholder != "Select option:" {
		t.Errorf("Expected placeholder %q, got %q", "Select option:", sel.placeholder)
	}
	if len(sel.options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(sel.options))
	}
	if sel.checked != -1 {
		t.Errorf("Expected checked to be -1, got %d", sel.checked)
	}
}

func TestSelector_UpdateUp(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	sel.cursor = 1
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyUp, Runes: []rune("up")})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector type")
	}
	if updatedSel.cursor != 0 {
		t.Errorf("Expected cursor to be 0, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateDown(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	// Initially cursor is 0.
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyDown, Runes: []rune("down")})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector type")
	}
	if updatedSel.cursor != 1 {
		t.Errorf("Expected cursor to be 1, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateEnter(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client", "server"})
	sel.cursor = 0
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyEnter, Runes: []rune("enter")})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector type")
	}
	expectedChoice := "client" // from "client mode"
	if updatedSel.choice != expectedChoice {
		t.Errorf("Expected choice %q, got %q", expectedChoice, updatedSel.choice)
	}
	if updatedSel.checked != 0 {
		t.Errorf("Expected checked to be 0, got %d", updatedSel.checked)
	}
	if cmd == nil {
		t.Error("Expected a quit command when pressing enter")
	}
}

func TestSelector_UpdateQ(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	_, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector type")
	}
	if cmd == nil {
		t.Error("Expected a quit command when pressing q")
	}
}

func TestSelector_View(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	sel.cursor = 0
	view := sel.View()
	if !strings.Contains(view, sel.placeholder) {
		t.Errorf("View output does not contain placeholder %q", sel.placeholder)
	}
	ansiStart := "\033[1;32m"
	ansiEnd := "\033[0m"
	if !strings.Contains(view, ansiStart) || !strings.Contains(view, ansiEnd) {
		t.Error("Expected ANSI escape codes for highlighted option")
	}
	sel.checked = 0
	view = sel.View()
	if !strings.Contains(view, "[x]") {
		t.Error("Expected first option to show as checked ([x])")
	}
}

func TestSelector_Choice(t *testing.T) {
	sel := NewSelector("Select option:", []string{"client mode", "server mode"})
	if sel.Choice() != "" {
		t.Errorf("Expected empty choice, got %q", sel.Choice())
	}
	sel.choice = "client"
	if sel.Choice() != "client" {
		t.Errorf("Expected choice to be 'client', got %q", sel.Choice())
	}
}
