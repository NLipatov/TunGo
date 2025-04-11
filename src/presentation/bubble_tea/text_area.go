package bubble_tea

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type TextArea struct {
	ta *textarea.Model
}

func NewTextArea(placeholder string) *TextArea {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(10)
	ta.ShowLineNumbers = true
	ta.Focus()
	return &TextArea{
		ta: &ta,
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
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	*m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *TextArea) View() string {
	return m.ta.View()
}
