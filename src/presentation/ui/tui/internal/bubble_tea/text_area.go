package bubble_tea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// TextArea - is a multiline text input
type TextArea struct {
	ta          *textarea.Model
	done        bool
	cancelled   bool
	placeholder string
	width       int
	height      int
}

func NewTextArea(placeholder string) *TextArea {
	ta := textarea.New()
	ta.Prompt = "> "
	ta.Placeholder = placeholder
	ta.SetWidth(80)
	ta.SetHeight(10)
	ta.ShowLineNumbers = true
	ta.Focus()
	return &TextArea{
		ta:          &ta,
		done:        false,
		placeholder: placeholder,
	}
}

func (m *TextArea) Value() string {
	return m.ta.Value()
}

func (m *TextArea) Cancelled() bool {
	return m.cancelled
}

func (m *TextArea) Init() tea.Cmd {
	return textarea.Blink
}

func (m *TextArea) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := contentWidthForTerminal(msg.Width)
		available := maxInt(1, contentWidth-inputContainerStyle().GetHorizontalFrameSize())
		m.ta.SetWidth(minInt(80, available))
		if msg.Height > 18 {
			m.ta.SetHeight(msg.Height - 18)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "enter" {
			m.done = true
			return m, tea.Quit
		}
		if msg.String() == "esc" {
			m.done = true
			m.cancelled = true
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
	value := m.ta.Value()
	lineCount := 1
	if value != "" {
		lineCount = len(strings.Split(value, "\n"))
	}
	stats := metaTextStyle().Render(fmt.Sprintf("Lines: %d", lineCount))
	container := inputContainerStyle().Width(m.inputContainerWidth())

	body := []string{
		container.Render(m.ta.View()),
		stats,
	}
	return renderScreen(
		m.width,
		m.height,
		"Paste configuration",
		m.placeholder,
		body,
		"Enter confirm | Esc Back",
	)
}

func (m *TextArea) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return maxInt(1, m.ta.Width()+inputContainerStyle().GetHorizontalFrameSize())
}
