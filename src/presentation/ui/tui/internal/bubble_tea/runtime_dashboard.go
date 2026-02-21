package bubble_tea

import (
	"context"
	"errors"
	"strings"
	"time"
	"tungo/infrastructure/telemetry/trafficstats"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
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

type runtimeTickMsg struct {
	seq uint64
}

type runtimeLogTickMsg struct {
	seq uint64
}
type runtimeContextDoneMsg struct {
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
	ctx                  context.Context
	mode                 RuntimeDashboardMode
	width                int
	height               int
	keys                 selectorKeyMap
	screen               runtimeDashboardScreen
	settingsCursor       int
	preferences          UIPreferences
	logFeed              RuntimeLogFeed
	logViewport          viewport.Model
	logReady             bool
	logFollow            bool
	logScratch           []string
	logWaitStop          chan struct{}
	rxSamples            [runtimeSparklinePoints]uint64
	txSamples            [runtimeSparklinePoints]uint64
	sampleCount          int
	sampleCursor         int
	tickSeq              uint64
	logTickSeq           uint64
	confirmOpen          bool
	confirmCursor        int
	runtimeSeq           uint64
	exitRequested        bool
	reconfigureRequested bool
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
	model := RuntimeDashboard{
		ctx:         ctx,
		mode:        mode,
		keys:        defaultSelectorKeyMap(),
		screen:      runtimeScreenDataplane,
		preferences: CurrentUIPreferences(),
		logFeed:     options.LogFeed,
		logViewport: viewport.New(1, 8),
		logReady:    true,
		logFollow:   true,
		tickSeq:     1,
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
	if finalModel.exitRequested {
		return false, ErrRuntimeDashboardExitRequested
	}
	return finalModel.reconfigureRequested, nil
}

func (m RuntimeDashboard) Init() tea.Cmd {
	return tea.Batch(
		runtimeTickCmd(m.tickSeq),
		waitForRuntimeContextDone(m.ctx, m.runtimeSeq),
	)
}

func (m RuntimeDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.screen == runtimeScreenLogs {
			m.refreshLogs()
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
	case runtimeLogTickMsg:
		if msg.seq != m.logTickSeq || m.screen != runtimeScreenLogs {
			return m, nil
		}
		m.refreshLogs()
		return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logWaitStop, m.logTickSeq, m.runtimeSeq)
	case runtimeContextDoneMsg:
		m.stopLogWait()
		return m, tea.Quit
	case tea.KeyMsg:
		if m.confirmOpen {
			return m.updateConfirm(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.stopLogWait()
			m.exitRequested = true
			return m, tea.Quit
		case msg.String() == "esc":
			switch m.screen {
			case runtimeScreenDataplane:
				m.confirmOpen = true
				m.confirmCursor = 0
			case runtimeScreenLogs:
				m.stopLogWait()
				m.screen = runtimeScreenDataplane
				return m, nil
			case runtimeScreenSettings:
				m.screen = runtimeScreenDataplane
			}
			return m, nil
		case key.Matches(msg, m.keys.Tab):
			previous := m.screen
			m.screen = m.nextScreen()
			m.preferences = CurrentUIPreferences()
			if m.screen == runtimeScreenLogs {
				m.restartLogWait()
				m.logTickSeq++
				m.refreshLogs()
				return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logWaitStop, m.logTickSeq, m.runtimeSeq)
			}
			if previous == runtimeScreenLogs {
				m.stopLogWait()
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

func (m RuntimeDashboard) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.stopLogWait()
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
			m.stopLogWait()
			m.reconfigureRequested = true
			return m, tea.Quit
		}
		m.confirmOpen = false
		m.confirmCursor = 0
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
	var cmd tea.Cmd
	prevGraphEnabled := m.preferences.ShowDataplaneGraph
	switch {
	case key.Matches(msg, m.keys.Up):
		m.settingsCursor = settingsCursorUp(m.settingsCursor)
	case key.Matches(msg, m.keys.Down):
		m.settingsCursor = settingsCursorDown(m.settingsCursor)
	case key.Matches(msg, m.keys.Left):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settingsCursor, -1)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	case key.Matches(msg, m.keys.Right), key.Matches(msg, m.keys.Select):
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settingsCursor, 1)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	}
	m.handleGraphPreferenceChange(prevGraphEnabled)
	return m, cmd
}

func (m *RuntimeDashboard) refreshLogs() {
	lines := runtimeLogSnapshot(m.logFeed, &m.logScratch)
	m.ensureLogsViewport()
	wasAtBottom := m.logViewport.AtBottom()
	offset := m.logViewport.YOffset
	content := renderLogsViewportContent(lines, m.logViewport.Width, resolveUIStyles(m.preferences))
	m.logViewport.SetContent(content)
	if m.logFollow || wasAtBottom {
		m.logViewport.GotoBottom()
		m.logFollow = true
		return
	}
	m.logViewport.SetYOffset(offset)
}

func (m RuntimeDashboard) mainView() string {
	styles := resolveUIStyles(m.preferences)
	title := m.tabsLine(styles)
	modeLine := "Mode: Client"
	status := "Status: Connected"
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
		body = append(body, "", "Stop tunnel and reconfigure?", "")
		body = append(body, renderSelectableRows(
			[]string{"Stay", "Stop tunnel and reconfigure"},
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
	)
}

func (m RuntimeDashboard) settingsView() string {
	styles := resolveUIStyles(m.preferences)
	body := []string{}
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body = append(body, renderSelectableRows(uiSettingsRows(m.preferences), m.settingsCursor, contentWidth, styles)...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		"up/k down/j row | left/right/Enter change | Tab switch tabs | ctrl+c exit",
	)
}

func (m RuntimeDashboard) logsView() string {
	styles := resolveUIStyles(m.preferences)
	body := []string{m.logViewport.View()}

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		m.logsHint(),
	)
}

func (m RuntimeDashboard) tabsLine(styles uiStyles) string {
	contentWidth := contentWidthForTerminal(m.width)
	return renderTabsLine(productLabel(), "runtime", runtimeTabs[:], int(m.screen), contentWidth, styles)
}

func (m *RuntimeDashboard) ensureLogsViewport() {
	contentWidth, viewportHeight := computeLogsViewportSize(
		m.width,
		m.height,
		m.preferences,
		"",
		m.logsHint(),
	)
	if !m.logReady {
		m.logViewport = viewport.New(contentWidth, viewportHeight)
		m.logReady = true
		return
	}
	m.logViewport.Width = contentWidth
	m.logViewport.Height = viewportHeight
}

func (m RuntimeDashboard) logsHint() string {
	return "up/down scroll | PgUp/PgDn page | Home/End jump | Space follow | Tab switch tabs | Esc back | ctrl+c exit"
}

func (m RuntimeDashboard) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyPgUp:
		m.logViewport.PageUp()
		m.logFollow = false
		return m, nil
	case tea.KeyPgDown:
		m.logViewport.PageDown()
		m.logFollow = m.logViewport.AtBottom()
		return m, nil
	case tea.KeyHome:
		m.logViewport.GotoTop()
		m.logFollow = false
		return m, nil
	case tea.KeyEnd:
		m.logViewport.GotoBottom()
		m.logFollow = true
		return m, nil
	case tea.KeySpace:
		m.logFollow = !m.logFollow
		if m.logFollow {
			m.logViewport.GotoBottom()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Up):
		m.logViewport.LineUp(1)
		m.logFollow = false
	case key.Matches(msg, m.keys.Down):
		m.logViewport.LineDown(1)
		m.logFollow = m.logViewport.AtBottom()
	}
	return m, nil
}

func runtimeTickCmd(seq uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return runtimeTickMsg{seq: seq}
	})
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
					return runtimeLogTickMsg{}
				case <-changes:
					return runtimeLogTickMsg{seq: logSeq}
				}
			}
		}
	}
	return runtimeLogTickCmd(logSeq)
}

func runtimeLogTickCmd(seq uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return runtimeLogTickMsg{seq: seq}
	})
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

func (m *RuntimeDashboard) restartLogWait() {
	m.stopLogWait()
	m.logWaitStop = make(chan struct{})
}

func (m *RuntimeDashboard) stopLogWait() {
	if m.logWaitStop != nil {
		close(m.logWaitStop)
		m.logWaitStop = nil
	}
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

	pixelWidth := maxInt(2, width*2)
	lastPos := maxInt(1, displayCount-1)
	var cellBuf [runtimeSparklinePoints]uint8
	cells := cellBuf[:width]
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
	return string(runeBuf[:width])
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
