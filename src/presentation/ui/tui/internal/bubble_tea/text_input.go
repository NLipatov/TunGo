package bubble_tea

import (
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TextInput - is a single line text input
type TextInput struct {
	ti          textinput.Model
	placeholder string
	width       int
	height      int
	cancelled   bool
}

func NewTextInput(placeholder string) *TextInput {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40
	return &TextInput{
		ti:          ti,
		placeholder: placeholder,
	}
}

func (m *TextInput) Value() string {
	return m.ti.Value()
}

func (m *TextInput) Cancelled() bool {
	return m.cancelled
}

func (m *TextInput) Init() tea.Cmd {
	return textinput.Blink
}

func (m *TextInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := contentWidthForTerminal(msg.Width)
		available := maxInt(1, contentWidth-inputContainerStyle().GetHorizontalFrameSize())
		// Keep stable text-input width to avoid visual jumps on first typed symbol.
		m.ti.Width = minInt(40, available)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "enter" {
			return m, tea.Quit
		}
		if msg.String() == "esc" {
			m.cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *TextInput) View() string {
	prefs := CurrentUIPreferences()
	styles := resolveUIStyles(prefs)
	container := inputContainerStyle().Width(m.inputContainerWidth())
	stats := metaTextStyle().Render("Characters: " + formatCount(utf8.RuneCountInString(m.ti.Value()), m.ti.CharLimit))
	body := []string{
		container.Render(m.ti.View()),
		stats,
	}
	return renderScreen(
		m.width,
		m.height,
		"Name configuration",
		m.placeholder,
		body,
		"Enter confirm | Esc Back",
		prefs,
		styles,
	)
}

func (m *TextInput) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return maxInt(1, m.ti.Width+inputContainerStyle().GetHorizontalFrameSize())
}
