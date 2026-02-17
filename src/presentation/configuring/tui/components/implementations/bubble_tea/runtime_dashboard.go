package bubble_tea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"tungo/infrastructure/telemetry/trafficstats"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type RuntimeDashboard struct {
	ctx            context.Context
	mode           RuntimeDashboardMode
	width          int
	height         int
	keys           selectorKeyMap
	screen         selectorScreen
	settingsCursor int
	preferences    UIPreferences
	logFeed        RuntimeLogFeed
	logLines       []string
	quitRequested  bool
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
		screen:      selectorScreenMain,
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
	program := tea.NewProgram(model, tea.WithAltScreen())
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
			if m.screen == selectorScreenMain {
				m.screen = selectorScreenSettings
			} else {
				m.screen = selectorScreenMain
			}
			m.preferences = CurrentUIPreferences()
			return m, nil
		}

		switch m.screen {
		case selectorScreenSettings:
			return m.updateSettings(msg)
		default:
			return m, nil
		}
	}
	return m, nil
}

func (m RuntimeDashboard) View() string {
	switch m.screen {
	case selectorScreenSettings:
		return m.settingsView()
	default:
		return m.mainView()
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
	limit := 8
	if m.height > 0 {
		limit = maxInt(4, minInt(14, m.height/3))
	}
	m.logLines = m.logFeed.Tail(limit)
}

func (m RuntimeDashboard) mainView() string {
	title := m.tabsLine()
	subtitle := "Client runtime • Traffic is routed through TunGo"
	status := "connected"
	if m.mode == RuntimeDashboardServer {
		subtitle = "Server runtime • Workers are running"
		status = "running"
	}

	snapshot := trafficstats.SnapshotGlobal()
	body := []string{
		optionTextStyle().Render(fmt.Sprintf("Status: %s", status)),
	}
	if !m.preferences.ShowFooter {
		body = append(body,
			optionTextStyle().Render(fmt.Sprintf("RX %s | TX %s", formatRateForPrefs(m.preferences, snapshot.RXRate), formatRateForPrefs(m.preferences, snapshot.TXRate))),
			optionTextStyle().Render(fmt.Sprintf("Total RX %s | TX %s", formatTotalForPrefs(m.preferences, snapshot.RXBytesTotal), formatTotalForPrefs(m.preferences, snapshot.TXBytesTotal))),
		)
	}
	body = append(body, "", optionTextStyle().Render("Recent logs"))

	if len(m.logLines) == 0 {
		body = append(body, metaTextStyle().Render("  waiting for logs..."))
	} else {
		for _, line := range m.logLines {
			body = append(body, metaTextStyle().Render("  "+line))
		}
	}

	return renderScreen(
		m.width,
		m.height,
		title,
		subtitle,
		body,
		"Tab switch Dataplane/Settings • q quit",
	)
}

func (m RuntimeDashboard) settingsView() string {
	body := []string{}

	rows := []string{
		fmt.Sprintf("Theme      : %s", strings.ToUpper(string(m.preferences.Theme))),
		fmt.Sprintf("Language   : %s", strings.ToUpper(m.preferences.Language)),
		fmt.Sprintf("Stats units: %s", strings.ToUpper(string(m.preferences.StatsUnits))),
		fmt.Sprintf("Show footer: %s", onOff(m.preferences.ShowFooter)),
	}

	for i, row := range rows {
		prefix := "  "
		if i == m.settingsCursor {
			prefix = "▸ "
			body = append(body, activeOptionTextStyle().Render(prefix+row))
			continue
		}
		body = append(body, optionTextStyle().Render(prefix+row))
	}
	body = append(body, "")
	body = append(body, optionTextStyle().Render("Language is MVP (EN only)."))

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(),
		"Settings • Theme, language, units, and footer preferences",
		body,
		"↑/k ↓/j row • ←/→/Enter change • Tab switch Dataplane/Settings • q quit",
	)
}

func (m RuntimeDashboard) tabsLine() string {
	label := headerLabelStyle().Render("TunGo")
	dataplaneTab := optionTextStyle().Render(" Dataplane ")
	settingsTab := optionTextStyle().Render(" Settings ")

	if m.screen == selectorScreenMain {
		dataplaneTab = activeOptionTextStyle().Render(" Dataplane ")
	} else {
		settingsTab = activeOptionTextStyle().Render(" Settings ")
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, label, "  ", dataplaneTab, " ", settingsTab)
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
