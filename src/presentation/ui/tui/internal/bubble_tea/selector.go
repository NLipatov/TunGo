package bubble_tea

import (
	"strings"
	"tungo/presentation/ui/tui/internal/ui/contracts/colorization"
	"tungo/presentation/ui/tui/internal/ui/value_objects"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type selectorKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Tab    key.Binding
	Select key.Binding
	Quit   key.Binding
}

func defaultSelectorKeyMap() selectorKeyMap {
	return selectorKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down/j", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("left/h", "previous"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("right/l", "next"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "settings"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "apply/select"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "exit"),
		),
	}
}

func (k selectorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Tab, k.Quit}
}

func (k selectorKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Select, k.Tab, k.Quit},
	}
}

type selectorScreen int

const (
	selectorScreenMain selectorScreen = iota
	selectorScreenSettings
	selectorScreenLogs
)

type Selector struct {
	settings                         *uiPreferencesProvider
	colorizer                        colorization.Colorizer
	foregroundColor, backgroundColor value_objects.Color
	placeholder                      string
	options                          []string
	cursor                           int
	choice                           string
	done                             bool
	width                            int
	height                           int
	keys                             selectorKeyMap
	screen                           selectorScreen
	settingsCursor                   int
	preferences                      UIPreferences
	logs                             logViewport
	backRequested                    bool
	quitRequested                    bool
}

func NewSelector(
	placeholder string,
	choices []string,
	colorizer colorization.Colorizer,
	foregroundColor, backgroundColor value_objects.Color,
	settings *uiPreferencesProvider,
) Selector {
	return Selector{
		settings:        settings,
		placeholder:     placeholder,
		options:         choices,
		colorizer:       colorizer,
		foregroundColor: foregroundColor,
		backgroundColor: backgroundColor,
		keys:            defaultSelectorKeyMap(),
		screen:          selectorScreenMain,
		preferences:     settings.Preferences(),
		logs:            newLogViewport(),
	}
}

func (m Selector) Choice() string {
	return m.choice
}

func (m Selector) BackRequested() bool {
	return m.backRequested
}

func (m Selector) QuitRequested() bool {
	return m.quitRequested
}

func (m Selector) Init() tea.Cmd {
	return nil
}

func (m Selector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.screen == selectorScreenLogs {
			m.logs.ensure(m.width, m.height, m.preferences, "", m.logsHint())
			m.logs.refresh(m.logsFeed(), m.preferences)
		}
	case logViewportTickMsg:
		if msg.seq != m.logs.tickSeq || m.screen != selectorScreenLogs {
			return m, nil
		}
		m.logs.refresh(m.logsFeed(), m.preferences)
		return m, selectorLogUpdateCmd(m.logsFeed(), m.logs.waitStop, m.logs.tickSeq)
	case tea.KeyPressMsg:
		switch {
		case msg.String() == "esc":
			m.logs.stopWait()
			m.backRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Quit):
			m.logs.stopWait()
			m.quitRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Tab):
			previous := m.screen
			m.screen = m.nextScreen()
			m.preferences = m.settings.Preferences()
			if m.screen == selectorScreenLogs {
				m.logs.restartWait()
				m.logs.tickSeq++
				m.logs.ensure(m.width, m.height, m.preferences, "", m.logsHint())
				m.logs.refresh(m.logsFeed(), m.preferences)
				return m, selectorLogUpdateCmd(m.logsFeed(), m.logs.waitStop, m.logs.tickSeq)
			}
			if previous == selectorScreenLogs {
				m.logs.stopWait()
			}
			return m, nil
		}

		switch m.screen {
		case selectorScreenSettings:
			return m.updateSettings(msg)
		case selectorScreenLogs:
			return m.updateLogs(msg)
		default:
			return m.updateMain(msg)
		}
	}
	return m, nil
}

func (m Selector) updateMain(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 && !m.done {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.options)-1 && !m.done {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Select):
		if !m.done && len(m.options) > 0 {
			m.choice = m.options[m.cursor]
			m.done = true
		}
		m.logs.stopWait()
		return m, tea.Quit
	}
	return m, nil
}

func (m Selector) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case key.Matches(msg, m.keys.Up):
		m.settingsCursor = settingsCursorUp(m.settingsCursor)
	case key.Matches(msg, m.keys.Down):
		m.settingsCursor = settingsCursorDown(m.settingsCursor, settingsVisibleRowCount(m.preferences, false))
	case key.Matches(msg, m.keys.Left):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settings, m.settingsCursor, -1, false)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Select):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settings, m.settingsCursor, 1, false)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	}
	return m, cmd
}

func nextTheme(current ThemeOption, step int) ThemeOption {
	order := orderedThemeOptions[:]
	idx := 0
	for i, item := range order {
		if item == current {
			idx = i
			break
		}
	}
	if !isValidTheme(current) {
		idx = 0
	}
	if step > 0 {
		idx = (idx + 1) % len(order)
	} else {
		idx = (idx - 1 + len(order)) % len(order)
	}
	return order[idx]
}

func nextStatsUnits(current StatsUnitsOption, step int) StatsUnitsOption {
	order := []StatsUnitsOption{StatsUnitsBytes, StatsUnitsBiBytes}
	idx := 0
	for i, item := range order {
		if item == current {
			idx = i
			break
		}
	}
	if step > 0 {
		idx = (idx + 1) % len(order)
	} else {
		idx = (idx - 1 + len(order)) % len(order)
	}
	return order[idx]
}

func (m Selector) View() tea.View {
	if m.done {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	title, details := splitPlaceholder(m.placeholder)
	subtitle := ""
	preamble := make([]string, 0, len(details))
	if len(details) > 0 {
		subtitle = details[0]
		preamble = append(preamble, details[1:]...)
	}

	var content string
	if m.screen == selectorScreenSettings {
		content = m.settingsView(preamble)
	} else if m.screen == selectorScreenLogs {
		content = m.logsView()
	} else {
		content = m.mainView(title, subtitle, preamble)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Selector) nextScreen() selectorScreen {
	switch m.screen {
	case selectorScreenMain:
		return selectorScreenSettings
	case selectorScreenSettings:
		return selectorScreenLogs
	default:
		return selectorScreenMain
	}
}

func (m Selector) mainView(title, subtitle string, preamble []string) string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	options := make([]string, 0, len(m.options))
	for i, choice := range m.options {
		pointer := "  "
		if m.cursor == i {
			pointer = "> "
		}
		line := truncateWithEllipsis(pointer+choice, contentWidth)
		if m.cursor == i {
			line = styles.active.Render(line)
		}
		options = append(options, line)
	}
	body := make([]string, 0, len(preamble)+len(options)+2)
	if strings.TrimSpace(subtitle) != "" {
		body = append(body, subtitle)
		body = append(body, "")
	}
	if len(preamble) > 0 {
		body = append(body, preamble...)
		body = append(body, "")
	}
	body = append(body, options...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		title,
		body,
		"up/k move | down/j move | Enter select | Tab switch tabs | Esc Back | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func (m Selector) settingsView(preamble []string) string {
	styles := resolveUIStyles(m.preferences)
	body := make([]string, 0, len(preamble)+8)
	if len(preamble) > 0 {
		body = append(body, preamble...)
		body = append(body, "")
	}

	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body = append(body, renderSelectableRows(uiSettingsRows(m.preferences, false), m.settingsCursor, contentWidth, styles)...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		"left/right or Enter change value | Tab switch tabs | Esc Back | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func (m Selector) logsView() string {
	styles := resolveUIStyles(m.preferences)
	body := []string{m.logs.view()}

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		m.logsHint(),
		m.preferences,
		styles,
	)
}

func (m Selector) logsFeed() RuntimeLogFeed {
	return GlobalRuntimeLogFeed()
}

func (m Selector) tabsLine(styles uiStyles) string {
	contentWidth := contentWidthForTerminal(m.width)
	return renderTabsLine(productLabel(), "selector", selectorTabs[:], int(m.screen), contentWidth, m.preferences.Theme, styles)
}

func (m Selector) logsHint() string {
	return "up/down scroll | PgUp/PgDn page | Home/End jump | Space follow | Tab switch tabs | Esc back | ctrl+c exit"
}

func (m Selector) updateLogs(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, m.logs.updateKeys(msg, m.keys)
}

func selectorLogUpdateCmd(feed RuntimeLogFeed, stop <-chan struct{}, seq uint64) tea.Cmd {
	return logViewportUpdateCmd(feed, stop, seq)
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func splitPlaceholder(raw string) (title string, details []string) {
	parts := strings.Split(raw, "\n")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return "Choose option", nil
	}
	if len(clean) == 1 {
		return clean[0], nil
	}
	return clean[0], clean[1:]
}
