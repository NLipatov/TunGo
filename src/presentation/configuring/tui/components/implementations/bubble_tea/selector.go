package bubble_tea

import (
	"fmt"
	"strings"
	"tungo/presentation/configuring/tui/components/domain/contracts/colorization"
	"tungo/presentation/configuring/tui/components/domain/value_objects"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type selectorKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Tab    key.Binding
	Select key.Binding
	Help   key.Binding
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
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "more"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "exit"),
		),
	}
}

func (k selectorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Tab, k.Quit}
}

func (k selectorKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Select, k.Tab, k.Help, k.Quit},
	}
}

type selectorScreen int

const (
	selectorScreenMain selectorScreen = iota
	selectorScreenSettings
	selectorScreenLogs
)

const (
	settingsThemeRow = iota
	settingsLanguageRow
	settingsStatsUnitsRow
	settingsFooterRow
	settingsRowsCount
)

type Selector struct {
	colorizer                        colorization.Colorizer
	foregroundColor, backgroundColor value_objects.Color
	placeholder                      string
	options                          []string
	cursor                           int
	choice                           string
	done                             bool
	width                            int
	height                           int
	help                             help.Model
	keys                             selectorKeyMap
	screen                           selectorScreen
	settingsCursor                   int
	preferences                      UIPreferences
	backRequested                    bool
	quitRequested                    bool
}

func NewSelector(
	placeholder string,
	choices []string,
	colorizer colorization.Colorizer,
	foregroundColor, backgroundColor value_objects.Color,
) Selector {
	return Selector{
		placeholder:     placeholder,
		options:         choices,
		colorizer:       colorizer,
		foregroundColor: foregroundColor,
		backgroundColor: backgroundColor,
		help:            help.New(),
		keys:            defaultSelectorKeyMap(),
		screen:          selectorScreenMain,
		preferences:     CurrentUIPreferences(),
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
		m.help.Width = maxInt(1, contentWidthForTerminal(msg.Width))
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case msg.String() == "esc":
			m.backRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Quit):
			m.quitRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Tab):
			m.screen = m.nextScreen()
			m.preferences = CurrentUIPreferences()
		}

		switch m.screen {
		case selectorScreenSettings:
			return m.updateSettings(msg)
		default:
			return m.updateMain(msg)
		}
	}
	return m, nil
}

func (m Selector) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if !m.done {
			m.choice = m.options[m.cursor]
			m.done = true
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m Selector) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.settingsCursor < settingsRowsCount-1 {
			m.settingsCursor++
		}
	case key.Matches(msg, m.keys.Left):
		m.preferences = m.changeSetting(-1)
	case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Select):
		m.preferences = m.changeSetting(1)
	}
	return m, nil
}

func (m Selector) changeSetting(step int) UIPreferences {
	return UpdateUIPreferences(func(p *UIPreferences) {
		switch m.settingsCursor {
		case settingsThemeRow:
			p.Theme = nextTheme(p.Theme, step)
		case settingsLanguageRow:
			// MVP: only English is supported right now.
			p.Language = "en"
		case settingsStatsUnitsRow:
			p.StatsUnits = nextStatsUnits(p.StatsUnits, step)
		case settingsFooterRow:
			p.ShowFooter = !p.ShowFooter
		}
	})
}

func nextTheme(current ThemeOption, step int) ThemeOption {
	order := []ThemeOption{ThemeLight, ThemeDark}
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

func (m Selector) View() string {
	if m.done {
		return ""
	}

	title, details := splitPlaceholder(m.placeholder)
	subtitle := ""
	preamble := make([]string, 0, len(details))
	if len(details) > 0 {
		subtitle = details[0]
		preamble = append(preamble, details[1:]...)
	}

	if m.screen == selectorScreenSettings {
		return m.settingsView(title, subtitle, preamble)
	}
	if m.screen == selectorScreenLogs {
		return m.logsView()
	}

	return m.mainView(title, subtitle, preamble)
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
		line := truncateWithEllipsis(fmt.Sprintf("%s%s", pointer, choice), contentWidth)
		if m.cursor == i {
			line = activeOptionTextStyle().Render(line)
		} else {
			line = optionTextStyle().Render(line)
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
		m.tabsLine(),
		title,
		body,
		"up/k move | down/j move | Enter select | Tab switch tabs | Esc Back | q exit",
	)
}

func (m Selector) settingsView(title, subtitle string, preamble []string) string {
	body := make([]string, 0, len(preamble)+8)
	if len(preamble) > 0 {
		body = append(body, preamble...)
		body = append(body, "")
	}

	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body = append(body, renderSelectableRows(uiSettingsRows(m.preferences), m.settingsCursor, contentWidth)...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(),
		"",
		body,
		"left/right or Enter change value | Tab switch tabs | Esc Back | q exit",
	)
}

func (m Selector) logsView() string {
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body := renderLogsBody(m.logsTail(), contentWidth)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(),
		"",
		body,
		"Tab switch tabs | Esc Back | q exit",
	)
}

func (m Selector) logsTail() []string {
	feed := GlobalRuntimeLogFeed()
	if feed == nil {
		return nil
	}
	return feed.Tail(logTailLimit(m.height))
}

func (m Selector) tabsLine() string {
	return renderTabsLine("TunGo", []string{"Main", "Settings", "Logs"}, int(m.screen))
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
