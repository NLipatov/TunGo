package bubble_tea

import (
	"context"
	"errors"
	"strings"
	"time"
	"tungo/infrastructure/telemetry/trafficstats"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type RuntimeDashboardMode string

const (
	RuntimeDashboardClient RuntimeDashboardMode = "client"
	RuntimeDashboardServer RuntimeDashboardMode = "server"
)

type RuntimeDashboardOptions struct {
	Mode            RuntimeDashboardMode
	LogFeed         RuntimeLogFeed
	ServerSupported bool
	ReadyCh         <-chan struct{}
}

type runtimeTickMsg struct {
	seq uint64
}

type runtimeContextDoneMsg struct {
	seq uint64
}

type runtimeReadyMsg struct {
	seq uint64
}

type runtimeDashboardScreen int

const (
	runtimeScreenDataplane runtimeDashboardScreen = iota
	runtimeScreenSettings
	runtimeScreenLogs
)

const (
	runtimeSparklinePoints = 40
)

var zeroBrailleSparklineCache = initZeroBrailleSparklineCache()

var ErrRuntimeDashboardExitRequested = errors.New("runtime dashboard exit requested")

type RuntimeDashboard struct {
	settings             *uiPreferencesProvider
	ctx                  context.Context
	mode                 RuntimeDashboardMode
	width                int
	height               int
	keys                 selectorKeyMap
	screen               runtimeDashboardScreen
	settingsCursor       int
	preferences          UIPreferences
	logFeed              RuntimeLogFeed
	logs                 logViewport
	rxSamples            [runtimeSparklinePoints]uint64
	txSamples            [runtimeSparklinePoints]uint64
	sampleCount          int
	sampleCursor         int
	serverSupported      bool
	tickSeq              uint64
	confirmOpen          bool
	confirmCursor        int
	runtimeSeq           uint64
	exitRequested        bool
	reconfigureRequested bool
	readyCh              <-chan struct{}
	connected            bool
}

type runtimeDashboardProgram interface {
	Run() (tea.Model, error)
}

var newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
	return tea.NewProgram(model)
}

func NewRuntimeDashboard(ctx context.Context, options RuntimeDashboardOptions, settings *uiPreferencesProvider) RuntimeDashboard {
	if ctx == nil {
		ctx = context.Background()
	}
	mode := options.Mode
	if mode != RuntimeDashboardServer {
		mode = RuntimeDashboardClient
	}
	readyCh := options.ReadyCh
	if readyCh == nil {
		ch := make(chan struct{})
		close(ch)
		readyCh = ch
	}
	connected := mode == RuntimeDashboardServer
	if !connected {
		select {
		case <-readyCh:
			connected = true
		default:
		}
	}
	model := RuntimeDashboard{
		settings:        settings,
		ctx:             ctx,
		mode:            mode,
		serverSupported: options.ServerSupported,
		keys:            defaultSelectorKeyMap(),
		screen:          runtimeScreenDataplane,
		preferences:     settings.Preferences(),
		logFeed:         options.LogFeed,
		logs:            newLogViewport(),
		tickSeq:         1,
		readyCh:         readyCh,
		connected:       connected,
	}
	if model.preferences.ShowDataplaneGraph {
		model.recordTrafficSample(trafficstats.SnapshotGlobal())
	}
	return model
}

func RunRuntimeDashboard(ctx context.Context, options RuntimeDashboardOptions) (reconfigure bool, err error) {
	defer clearTerminalAfterTUI()

	safeCtx := ctx
	if safeCtx == nil {
		safeCtx = context.Background()
	}
	settings := loadUISettingsFromDisk()
	model := NewRuntimeDashboard(safeCtx, options, settings)
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
	finalModel.logs.stopWait()
	if finalModel.exitRequested {
		return false, ErrRuntimeDashboardExitRequested
	}
	return finalModel.reconfigureRequested, nil
}

func (m RuntimeDashboard) Init() tea.Cmd {
	cmds := []tea.Cmd{
		runtimeTickCmd(m.tickSeq),
		waitForRuntimeContextDone(m.ctx, m.runtimeSeq),
	}
	if m.mode == RuntimeDashboardClient && !m.connected {
		cmds = append(cmds, waitForReadyCh(m.ctx, m.readyCh, m.runtimeSeq))
	}
	return tea.Batch(cmds...)
}

func (m RuntimeDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.screen == runtimeScreenLogs {
			m.logs.ensure(m.width, m.height, m.preferences, "", m.logsHint())
			m.logs.refresh(m.logFeed, m.preferences)
		}
		return m, nil
	case runtimeTickMsg:
		if msg.seq != m.tickSeq {
			return m, nil
		}
		if m.screen != runtimeScreenDataplane {
			return m, nil
		}
		if m.preferences.ShowDataplaneGraph {
			m.recordTrafficSample(trafficstats.SnapshotGlobal())
		}
		return m, runtimeTickCmd(m.tickSeq)
	case logViewportTickMsg:
		if msg.seq != m.logs.tickSeq || m.screen != runtimeScreenLogs {
			return m, nil
		}
		m.logs.refresh(m.logFeed, m.preferences)
		return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logs.waitStop, m.logs.tickSeq, m.runtimeSeq)
	case runtimeReadyMsg:
		if msg.seq == m.runtimeSeq {
			m.connected = true
		}
		return m, nil
	case runtimeContextDoneMsg:
		m.logs.stopWait()
		return m, tea.Quit
	case tea.KeyPressMsg:
		if m.confirmOpen {
			return m.updateConfirm(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.logs.stopWait()
			m.exitRequested = true
			return m, tea.Quit
		case msg.String() == "esc":
			switch m.screen {
			case runtimeScreenDataplane:
				if m.mode == RuntimeDashboardClient && !m.connected {
					m.logs.stopWait()
					m.reconfigureRequested = true
					return m, tea.Quit
				}
				m.confirmOpen = true
				m.confirmCursor = 0
			case runtimeScreenLogs:
				m.logs.stopWait()
				m.screen = runtimeScreenDataplane
				m.tickSeq++
				return m, runtimeTickCmd(m.tickSeq)
			case runtimeScreenSettings:
				m.screen = runtimeScreenDataplane
				m.tickSeq++
				return m, runtimeTickCmd(m.tickSeq)
			}
			return m, nil
		case key.Matches(msg, m.keys.Tab):
			previous := m.screen
			m.screen = m.nextScreen()
			m.preferences = m.settings.Preferences()
			if m.screen == runtimeScreenLogs {
				m.logs.restartWait()
				m.logs.tickSeq++
				m.logs.ensure(m.width, m.height, m.preferences, "", m.logsHint())
				m.logs.refresh(m.logFeed, m.preferences)
				return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logs.waitStop, m.logs.tickSeq, m.runtimeSeq)
			}
			if previous == runtimeScreenLogs {
				m.logs.stopWait()
			}
			if m.screen == runtimeScreenDataplane && previous != runtimeScreenDataplane {
				m.tickSeq++
				return m, runtimeTickCmd(m.tickSeq)
			}
			return m, nil
		}

		switch m.screen {
		case runtimeScreenSettings:
			return m.updateSettings(msg)
		case runtimeScreenLogs:
			return m.updateLogs(msg)
		default:
			return m, nil
		}
	}
	return m, nil
}

func (m RuntimeDashboard) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.logs.stopWait()
		m.exitRequested = true
		return m, tea.Quit
	case msg.String() == "esc":
		m.confirmOpen = false
		m.confirmCursor = 0
		return m, nil
	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Left):
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
	case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Right):
		if m.confirmCursor < 1 {
			m.confirmCursor++
		}
	case key.Matches(msg, m.keys.Select):
		if m.confirmCursor == 1 {
			m.logs.stopWait()
			m.reconfigureRequested = true
			return m, tea.Quit
		}
		m.confirmOpen = false
		m.confirmCursor = 0
	}
	return m, nil
}

func (m RuntimeDashboard) View() tea.View {
	var content string
	switch m.screen {
	case runtimeScreenSettings:
		content = m.settingsView()
	case runtimeScreenLogs:
		content = m.logsView()
	default:
		content = m.mainView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
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

func (m RuntimeDashboard) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	prevGraphEnabled := m.preferences.ShowDataplaneGraph
	switch {
	case key.Matches(msg, m.keys.Up):
		m.settingsCursor = settingsCursorUp(m.settingsCursor)
	case key.Matches(msg, m.keys.Down):
		m.settingsCursor = settingsCursorDown(m.settingsCursor, settingsVisibleRowCount(m.preferences, m.serverSupported))
	case key.Matches(msg, m.keys.Left):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settings, m.settingsCursor, -1, m.serverSupported)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Select):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settings, m.settingsCursor, 1, m.serverSupported)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	}
	m.handleGraphPreferenceChange(prevGraphEnabled)
	return m, cmd
}

func (m RuntimeDashboard) mainView() string {
	styles := resolveUIStyles(m.preferences)
	title := m.tabsLine(styles)
	modeLine := "Mode: Client"
	status := "Status: Connecting to server..."
	if m.connected {
		status = "Status: Connected"
	}
	if m.mode == RuntimeDashboardServer {
		modeLine = "Mode: Server"
		status = "Status: Running"
	}
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	body := []string{
		modeLine,
		status,
	}
	if m.preferences.ShowDataplaneStats || m.preferences.ShowDataplaneGraph {
		body = append(body, "")
	}
	if m.preferences.ShowDataplaneStats {
		snapshot := trafficstats.SnapshotGlobal()
		statsLines := formatStatsLines(m.preferences, snapshot)
		body = append(body, statsLines[0], statsLines[1])
	}
	if m.preferences.ShowDataplaneGraph {
		sparklineWidth := sparklineWidthForContent(contentWidth)
		body = append(
			body,
			"RX trend: "+renderRateBrailleRing(m.rxSamples, m.sampleCount, m.sampleCursor, sparklineWidth),
			"TX trend: "+renderRateBrailleRing(m.txSamples, m.sampleCount, m.sampleCursor, sparklineWidth),
		)
	}
	if !m.preferences.ShowDataplaneStats && !m.preferences.ShowDataplaneGraph {
		body = append(body, "", "Dataplane metrics are hidden in Settings.")
	}
	if m.confirmOpen {
		body = append(body, "", "Stop tunnel?", "")
		body = append(body, renderSelectableRows(
			[]string{"Continue", "Stop"},
			m.confirmCursor,
			contentWidth,
			styles,
		)...)
	}
	hint := "Tab switch tabs | ctrl+c exit"
	if m.confirmOpen {
		hint = "left/right choose | Enter confirm | Esc cancel | ctrl+c exit"
	}

	return renderScreenRaw(
		m.width,
		m.height,
		title,
		"",
		body,
		hint,
		m.preferences,
		styles,
	)
}

func (m RuntimeDashboard) settingsView() string {
	styles := resolveUIStyles(m.preferences)
	body := []string{}
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body = append(body, renderSelectableRows(uiSettingsRows(m.preferences, m.serverSupported), m.settingsCursor, contentWidth, styles)...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		"up/k down/j row | left/right/Enter change | Tab switch tabs | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func (m RuntimeDashboard) logsView() string {
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

func (m RuntimeDashboard) tabsLine(styles uiStyles) string {
	contentWidth := contentWidthForTerminal(m.width)
	return renderTabsLine(productLabel(), "runtime", runtimeTabs[:], int(m.screen), contentWidth, m.preferences.Theme, styles)
}

func (m RuntimeDashboard) logsHint() string {
	return "up/down scroll | PgUp/PgDn page | Home/End jump | Space follow | Tab switch tabs | Esc back | ctrl+c exit"
}

func (m RuntimeDashboard) updateLogs(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, m.logs.updateKeys(msg, m.keys)
}

func runtimeTickCmd(seq uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return runtimeTickMsg{seq: seq}
	})
}

func waitForReadyCh(ctx context.Context, ch <-chan struct{}, seq uint64) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return runtimeContextDoneMsg{seq: seq}
		case <-ch:
			return runtimeReadyMsg{seq: seq}
		}
	}
}

func runtimeLogUpdateCmd(
	ctx context.Context,
	feed RuntimeLogFeed,
	stop <-chan struct{},
	logSeq uint64,
	runtimeSeq uint64,
) tea.Cmd {
	changeFeed, ok := feed.(RuntimeLogChangeFeed)
	if ok {
		changes := changeFeed.Changes()
		if changes != nil {
			return func() tea.Msg {
				select {
				case <-ctx.Done():
					return runtimeContextDoneMsg{seq: runtimeSeq}
				case <-stop:
					return logViewportTickMsg{}
				case <-changes:
					return logViewportTickMsg{seq: logSeq}
				}
			}
		}
	}
	return logViewportTickCmd(logSeq)
}

func waitForRuntimeContextDone(ctx context.Context, seq uint64) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return runtimeContextDoneMsg{seq: seq}
	}
}

func (m *RuntimeDashboard) recordTrafficSample(snapshot trafficstats.Snapshot) {
	m.rxSamples[m.sampleCursor] = snapshot.RXRate
	m.txSamples[m.sampleCursor] = snapshot.TXRate
	if m.sampleCount < runtimeSparklinePoints {
		m.sampleCount++
	}
	m.sampleCursor = (m.sampleCursor + 1) % runtimeSparklinePoints
}

func (m *RuntimeDashboard) handleGraphPreferenceChange(previous bool) {
	current := m.preferences.ShowDataplaneGraph
	if previous == current {
		return
	}
	if !current {
		m.clearTrafficSamples()
		return
	}
	m.recordTrafficSample(trafficstats.SnapshotGlobal())
}

func (m *RuntimeDashboard) clearTrafficSamples() {
	for i := range m.rxSamples {
		m.rxSamples[i] = 0
		m.txSamples[i] = 0
	}
	m.sampleCount = 0
	m.sampleCursor = 0
}

func sparklineWidthForContent(contentWidth int) int {
	if contentWidth <= 0 {
		return 20
	}
	available := contentWidth - len("RX trend: ")
	return maxInt(12, minInt(runtimeSparklinePoints, available))
}

func renderRateBrailleRing(
	samples [runtimeSparklinePoints]uint64,
	count, cursor, width int,
) string {
	if count <= 0 {
		return "no-data"
	}
	if width <= 0 {
		width = minInt(runtimeSparklinePoints, count)
	}
	displayCount := minInt(count, width)
	maxValue := uint64(0)
	for i := 0; i < displayCount; i++ {
		value := ringSampleAt(samples, displayCount, cursor, i)
		if value > maxValue {
			maxValue = value
		}
	}

	if maxValue == 0 {
		return zeroBrailleSparkline(width)
	}

	dataWidth := displayCount
	pixelWidth := maxInt(2, dataWidth*2)
	lastPos := maxInt(1, displayCount-1)
	var cellBuf [runtimeSparklinePoints]uint8
	cells := cellBuf[:dataWidth]
	lastY := -1
	for x := 0; x < pixelWidth; x++ {
		pos := (x * lastPos) / maxInt(1, pixelWidth-1)
		value := ringSampleAt(samples, displayCount, cursor, pos)
		y := brailleRow(value, maxValue)
		setBrailleDot(cells, x, y)
		if lastY >= 0 && lastY != y {
			start, end := lastY, y
			if start > end {
				start, end = end, start
			}
			for mid := start; mid <= end; mid++ {
				setBrailleDot(cells, x, mid)
			}
		}
		lastY = y
	}

	var runeBuf [runtimeSparklinePoints]rune
	for i, mask := range cells {
		runeBuf[i] = rune(0x2800 + int(mask))
	}
	padWidth := width - dataWidth
	if padWidth > 0 {
		return zeroBrailleSparkline(padWidth) + string(runeBuf[:dataWidth])
	}
	return string(runeBuf[:dataWidth])
}

func initZeroBrailleSparklineCache() [runtimeSparklinePoints + 1]string {
	var out [runtimeSparklinePoints + 1]string
	for i := 1; i <= runtimeSparklinePoints; i++ {
		out[i] = strings.Repeat("⣀", i)
	}
	return out
}

func zeroBrailleSparkline(width int) string {
	if width <= 0 {
		return ""
	}
	if width > runtimeSparklinePoints {
		width = runtimeSparklinePoints
	}
	return zeroBrailleSparklineCache[width]
}

func brailleRow(value, maxValue uint64) int {
	if maxValue == 0 {
		return 3
	}
	level := int((value * 3) / maxValue)
	return 3 - level
}

func setBrailleDot(cells []uint8, xPixel int, yRow int) {
	if len(cells) == 0 || xPixel < 0 {
		return
	}
	cellIndex := xPixel / 2
	if cellIndex < 0 || cellIndex >= len(cells) {
		return
	}
	subColumn := xPixel % 2
	if yRow < 0 {
		yRow = 0
	}
	if yRow > 3 {
		yRow = 3
	}
	cells[cellIndex] |= brailleDotMask(subColumn, yRow)
}

func brailleDotMask(subColumn int, yRow int) uint8 {
	if subColumn == 0 {
		switch yRow {
		case 0:
			return 1
		case 1:
			return 2
		case 2:
			return 4
		default:
			return 64
		}
	}
	switch yRow {
	case 0:
		return 8
	case 1:
		return 16
	case 2:
		return 32
	default:
		return 128
	}
}

func ringSampleAt(
	samples [runtimeSparklinePoints]uint64,
	count, cursor, pos int,
) uint64 {
	if count <= 0 || pos < 0 || pos >= count {
		return 0
	}
	start := (cursor - count + runtimeSparklinePoints) % runtimeSparklinePoints
	index := (start + pos) % runtimeSparklinePoints
	return samples[index]
}
