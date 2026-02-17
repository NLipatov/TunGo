package bubble_tea

import (
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextInput - is a single line text input
type TextInput struct {
	ti          textinput.Model
	placeholder string
	width       int
	height      int
}

func NewTextInput(placeholder string) *TextInput {
	ti := textinput.New()
	ti.Prompt = "┃ "
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40
	ti.PromptStyle = lipgloss.NewStyle().Foreground(themeColor("#00ADD8", "#00ADD8"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(themeColor("#000000", "#00ff66"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(themeColor("#4b5563", "#5fd18a"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(themeColor("#00ADD8", "#00ADD8"))
	return &TextInput{
		ti:          ti,
		placeholder: placeholder,
	}
}

func (m *TextInput) Value() string {
	return m.ti.Value()
}

func (m *TextInput) Init() tea.Cmd {
	return textinput.Blink
}

func (m *TextInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if msg.Width > 26 {
			m.ti.Width = msg.Width - 26
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "enter" {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *TextInput) View() string {
	stats := metaTextStyle().Render("Characters: " + formatCount(utf8.RuneCountInString(m.ti.Value()), m.ti.CharLimit))
	body := []string{
		inputContainerStyle().Render(m.ti.View()),
		stats,
	}
	return renderScreen(
		m.width,
		m.height,
		"Name configuration",
		m.placeholder,
		body,
		"Enter to confirm",
	)
}
