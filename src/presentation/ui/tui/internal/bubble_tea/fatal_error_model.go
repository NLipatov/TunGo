package bubble_tea

import tea "github.com/charmbracelet/bubbletea"

type fatalErrorModel struct {
	title   string
	message string
	width   int
	height  int
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
			return m, tea.Quit
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m fatalErrorModel) View() string {
	prefs := CurrentUIPreferences()
	styles := resolveUIStyles(prefs)
	return renderScreen(
		m.width,
		m.height,
		m.title,
		m.message,
		nil,
		"Press Enter to exit",
		prefs,
		styles,
	)
}
