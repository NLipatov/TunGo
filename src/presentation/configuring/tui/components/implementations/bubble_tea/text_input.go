package bubble_tea

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TextInput - is a single line text input
type TextInput struct {
	ti textinput.Model
}

func NewTextInput(placeholder string) *TextInput {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40
	return &TextInput{
		ti: ti,
	}
}

func (m *TextInput) Value() string {
	return m.ti.Value()
}

func (m *TextInput) Init() tea.Cmd {
	return textinput.Blink
}

func (m *TextInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m *TextInput) View() string {
	return m.ti.View()
}
