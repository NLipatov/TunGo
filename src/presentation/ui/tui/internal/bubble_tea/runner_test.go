package bubble_tea

import (
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type instantQuitModel struct{}

func (m instantQuitModel) Init() tea.Cmd                       { return tea.Quit }
func (m instantQuitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m instantQuitModel) View() string                        { return "" }

func TestBubbleProgramRunner_Run(t *testing.T) {
	r := &bubbleProgramRunner{}
	got, err := r.Run(instantQuitModel{}, tea.WithInput(nil), tea.WithOutput(io.Discard))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.(instantQuitModel); !ok {
		t.Fatalf("unexpected model type: %T", got)
	}
}
