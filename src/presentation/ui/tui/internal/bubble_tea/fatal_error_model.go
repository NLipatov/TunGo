package bubble_tea

import tea "github.com/charmbracelet/bubbletea"

type fatalErrorModel struct {
	title     string
	message   string
	width     int
	height    int
	dismissed bool
}

func newFatalErrorModel(title, message string) fatalErrorModel {
	return fatalErrorModel{
		title:   title,
		message: message,
	}
}

func (m fatalErrorModel) Init() tea.Cmd {
	return nil
}

func (m fatalErrorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEscape:
			m.dismissed = true
			return m, tea.Quit
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				m.dismissed = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// NewFatalErrorProgram creates a standalone tea.Program that displays
// a themed fatal error screen. The program blocks until the user dismisses it.
func NewFatalErrorProgram(title, message string) *tea.Program {
	return tea.NewProgram(newFatalErrorModel(title, message), tea.WithAltScreen())
}

func (m fatalErrorModel) View() string {
	prefs := CurrentUIPreferences()
	styles := resolveUIStyles(prefs)
	contentWidth := contentWidthForTerminal(m.width)
	tabsLine := renderTabsLine(
		productLabel(), "error", []string{"Error"}, 0,
		contentWidth, prefs.Theme, styles,
	)
	return renderScreen(
		m.width,
		m.height,
		tabsLine,
		m.title,
		wrapText(m.message, contentWidth),
		"Press Enter to exit",
		prefs,
		styles,
	)
}
