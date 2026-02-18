package bubble_tea

import (
	"context"
	"errors"
	"fmt"
	"time"
	"tungo/infrastructure/telemetry/trafficstats"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type RuntimeDashboardMode string

const (
	RuntimeDashboardClient RuntimeDashboardMode = "client"
	RuntimeDashboardServer RuntimeDashboardMode = "server"
)

type RuntimeDashboardOptions struct {
	Mode    RuntimeDashboardMode
	LogFeed RuntimeLogFeed
}

type runtimeTickMsg struct{}
type runtimeLogTickMsg struct{}
type runtimeContextDoneMsg struct{}

type runtimeDashboardScreen int

const (
	runtimeScreenDataplane runtimeDashboardScreen = iota
	runtimeScreenSettings
	runtimeScreenLogs
)

type RuntimeDashboard struct {
	ctx            context.Context
	mode           RuntimeDashboardMode
	width          int
	height         int
	keys           selectorKeyMap
	screen         runtimeDashboardScreen
	settingsCursor int
	preferences    UIPreferences
	logFeed        RuntimeLogFeed
	logLines       []string
	quitRequested  bool
}

type runtimeDashboardProgram interface {
	Run() (tea.Model, error)
}

var newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
	return tea.NewProgram(model, tea.WithAltScreen())
}

func NewRuntimeDashboard(ctx context.Context, options RuntimeDashboardOptions) RuntimeDashboard {
	if ctx == nil {
		ctx = context.Background()
	}
	mode := options.Mode
	if mode != RuntimeDashboardServer {
		mode = RuntimeDashboardClient
	}
	return RuntimeDashboard{
		ctx:         ctx,
		mode:        mode,
		keys:        defaultSelectorKeyMap(),
		screen:      runtimeScreenDataplane,
		preferences: CurrentUIPreferences(),
		logFeed:     options.LogFeed,
	}
}

func RunRuntimeDashboard(ctx context.Context, options RuntimeDashboardOptions) (bool, error) {
	defer clearTerminalAfterTUI()

	safeCtx := ctx
	if safeCtx == nil {
		safeCtx = context.Background()
	}
	model := NewRuntimeDashboard(safeCtx, options)
	program := newRuntimeDashboardProgram(model)
	result, err := program.Run()
	if err != nil {
		if errors.Is(safeCtx.Err(), context.Canceled) {
			return false, nil
		}
		return false, err
	}
	finalModel, ok := result.(RuntimeDashboard)
	if !ok {
		return false, nil
	}
	return finalModel.quitRequested, nil
}

func (m RuntimeDashboard) Init() tea.Cmd {
	return tea.Batch(
		runtimeTickCmd(),
		runtimeLogTickCmd(),
		waitForRuntimeContextDone(m.ctx),
	)
}

func (m RuntimeDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshLogs()
		return m, nil
	case runtimeTickMsg:
		return m, runtimeTickCmd()
	case runtimeLogTickMsg:
		m.refreshLogs()
		return m, runtimeLogTickCmd()
	case runtimeContextDoneMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitRequested = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Tab):
			m.screen = m.nextScreen()
			m.preferences = CurrentUIPreferences()
			return m, nil
		}

		switch m.screen {
		case runtimeScreenSettings:
			return m.updateSettings(msg)
		default:
			return m, nil
		}
	}
	return m, nil
}

func (m RuntimeDashboard) View() string {
	switch m.screen {
	case runtimeScreenSettings:
		return m.settingsView()
	case runtimeScreenLogs:
		return m.logsView()
	default:
		return m.mainView()
	}
}

func (m RuntimeDashboard) nextScreen() runtimeDashboardScreen {
	switch m.screen {
	case runtimeScreenDataplane:
		return runtimeScreenSettings
	case runtimeScreenSettings:
		return runtimeScreenLogs
	default:
		return runtimeScreenDataplane
	}
}

func (m RuntimeDashboard) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m RuntimeDashboard) changeSetting(step int) UIPreferences {
	return UpdateUIPreferences(func(p *UIPreferences) {
		switch m.settingsCursor {
		case settingsThemeRow:
			p.Theme = nextTheme(p.Theme, step)
		case settingsLanguageRow:
			p.Language = "en"
		case settingsStatsUnitsRow:
			p.StatsUnits = nextStatsUnits(p.StatsUnits, step)
		case settingsFooterRow:
			p.ShowFooter = !p.ShowFooter
		}
	})
}

func (m *RuntimeDashboard) refreshLogs() {
	if m.logFeed == nil {
		m.logLines = nil
		return
	}
	m.logLines = m.logFeed.Tail(logTailLimit(m.height))
}

func (m RuntimeDashboard) mainView() string {
	title := m.tabsLine()
	subtitle := "Client runtime - Traffic is routed through TunGo"
	status := "connected"
	if m.mode == RuntimeDashboardServer {
		subtitle = "Server runtime - Workers are running"
		status = "running"
	}

	snapshot := trafficstats.SnapshotGlobal()
	statsLines := formatStatsLines(m.preferences, snapshot)
	body := []string{
		optionTextStyle().Render(fmt.Sprintf("Status: %s", status)),
	}
	if !m.preferences.ShowFooter {
		for _, line := range statsLines {
			body = append(body, optionTextStyle().Render(line))
		}
	}
	body = append(body, "", metaTextStyle().Render("Open Logs tab for live runtime output."))

	return renderScreen(
		m.width,
		m.height,
		title,
		subtitle,
		body,
		"Tab switch tabs | q exit",
	)
}

func (m RuntimeDashboard) settingsView() string {
	body := []string{}
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
		"up/k down/j row | left/right/Enter change | Tab switch tabs | q exit",
	)
}

func (m RuntimeDashboard) logsView() string {
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body := renderLogsBody(m.logLines, contentWidth)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(),
		"",
		body,
		"Tab switch tabs | q exit",
	)
}

func (m RuntimeDashboard) tabsLine() string {
	return renderTabsLine("TunGo", []string{"Dataplane", "Settings", "Logs"}, int(m.screen))
}

func runtimeTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return runtimeTickMsg{}
	})
}

func runtimeLogTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return runtimeLogTickMsg{}
	})
}

func waitForRuntimeContextDone(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return runtimeContextDoneMsg{}
	}
}
