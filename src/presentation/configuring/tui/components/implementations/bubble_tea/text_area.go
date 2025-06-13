package bubble_tea

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// TextArea - is a multiline text input
type TextArea struct {
	ta   *textarea.Model
	done bool
}

func NewTextArea(placeholder string) *TextArea {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.SetWidth(80)
	ta.SetHeight(10)
	ta.ShowLineNumbers = true
	ta.Focus()
	return &TextArea{
		ta:   &ta,
		done: false,
	}
}

func (m *TextArea) Value() string {
	return m.ta.Value()
}

func (m *TextArea) Init() tea.Cmd {
	return textarea.Blink
}

func (m *TextArea) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	*m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *TextArea) View() string {
	if m.done {
		return ""
	}
	return m.ta.View()
}
