package bubble_tea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// TextArea - is a multiline text input
type TextArea struct {
	settings    UIPreferencesProvider
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
	ta.FocusedStyle.CursorLine = ta.FocusedStyle.Text
	ta.Focus()
	return &TextArea{
		settings:    loadUISettingsFromDisk(),
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
		available := maxInt(1, contentWidth-resolveUIStyles(m.settings.Preferences()).inputFrame.GetHorizontalFrameSize())
		m.ta.SetWidth(minInt(80, available))
		if msg.Height > 18 {
			m.ta.SetHeight(msg.Height - 18)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+d" {
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
	prefs := m.settings.Preferences()
	styles := resolveUIStyles(prefs)
	stats := styles.meta.Render(fmt.Sprintf("Lines: %d", lineCount))
	container := styles.inputFrame.Width(m.inputContainerWidth())

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
		"Ctrl+D confirm | Esc Back",
		prefs,
		styles,
	)
}

func (m *TextArea) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return maxInt(1, m.ta.Width()+resolveUIStyles(m.settings.Preferences()).inputFrame.GetHorizontalFrameSize())
}
