package bubble_tea

import (
	"context"
	"net/netip"
	"strings"
	"time"
	"tungo/internal/config"
	"tungo/internal/config/settings"
	"tungo/internal/trafficstats"

	tea "charm.land/bubbletea/v2"
)

type RuntimeDashboardOptions struct {
	Mode            config.Mode
	LogFeed         RuntimeLogFeed
	ServerSupported bool
	Ready           func() bool
	Protocol        settings.Protocol
	Endpoints       []config.EndpointInfo
}

type runtimeTickMsg struct {
	seq uint64
}

type runtimeContextDoneMsg struct{}

type runtimeDashboardScreen int

const (
	runtimeScreenDataplane runtimeDashboardScreen = iota
	runtimeScreenSettings
	runtimeScreenLogs
)

const (
	runtimeSparklinePoints = 40

	runtimeStopConfirmTitleClient = "Stop tunnel?"
	runtimeStopConfirmTitleServer = "Stop server?"

	runtimeHintDataplaneStopConfirm = "Esc open stop confirmation | Tab switch tabs | ctrl+c exit"
	runtimeHintDataplaneReconfigure = "Esc reconfigure | Tab switch tabs | ctrl+c exit"
	runtimeHintDataplaneConfirmOpen = "left/right choose | Enter confirm | Esc cancel | ctrl+c exit"
)

var zeroBrailleSparklineCache = initZeroBrailleSparklineCache()

type RuntimeDashboard struct {
	settings             *Preferences
	ctx                  context.Context
	mode                 config.Mode
	width                int
	height               int
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
	reconfigureRequested bool
	ready                func() bool
	connected            bool
	protocol             settings.Protocol
	endpoints            []config.EndpointInfo
}

func NewRuntimeDashboard(ctx context.Context, options RuntimeDashboardOptions, settings *Preferences) RuntimeDashboard {
	if ctx == nil {
		ctx = context.Background()
	}
	mode := options.Mode
	if mode != config.ModeServer {
		mode = config.ModeClient
	}
	ready := options.Ready
	if ready == nil {
		ready = func() bool { return true }
	}
	connected := mode == config.ModeServer || ready()
	model := RuntimeDashboard{
		settings:        settings,
		ctx:             ctx,
		mode:            mode,
		serverSupported: options.ServerSupported,
		screen:          runtimeScreenDataplane,
		preferences:     settings.Current(),
		logFeed:         options.LogFeed,
		logs:            newLogViewport(),
		tickSeq:         1,
		ready:           ready,
		connected:       connected,
		protocol:        options.Protocol,
		endpoints:       options.Endpoints,
	}
	if model.preferences.ShowDataplaneGraph {
		model.recordTrafficSample(trafficstats.SnapshotGlobal())
	}
	return model
}

func (m RuntimeDashboard) Init() tea.Cmd {
	return tea.Batch(
		runtimeTickCmd(m.tickSeq),
		waitForRuntimeContextDone(m.ctx),
	)
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
		if !m.connected && m.ready() {
			m.connected = true
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
		return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logs.waitStop, m.logs.tickSeq)
	case runtimeContextDoneMsg:
		m.logs.stopWait()
		return m, tea.Quit
	case tea.KeyPressMsg:
		if m.confirmOpen {
			return m.updateConfirm(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			m.logs.stopWait()
			return m, tea.Quit
		case "esc":
			switch m.screen {
			case runtimeScreenDataplane:
				if m.mode == config.ModeClient && !m.connected {
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
		case "tab":
			previous := m.screen
			m.screen = m.nextScreen()
			m.preferences = m.settings.Current()
			if m.screen == runtimeScreenLogs {
				m.logs.restartWait()
				m.logs.tickSeq++
				m.logs.ensure(m.width, m.height, m.preferences, "", m.logsHint())
				m.logs.refresh(m.logFeed, m.preferences)
				return m, runtimeLogUpdateCmd(m.ctx, m.logFeed, m.logs.waitStop, m.logs.tickSeq)
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
	switch msg.String() {
	case "ctrl+c":
		m.logs.stopWait()
		return m, tea.Quit
	case "esc":
		m.confirmOpen = false
		m.confirmCursor = 0
		return m, nil
	case "up", "k", "left", "h":
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
	case "down", "j", "right", "l":
		if m.confirmCursor < 1 {
			m.confirmCursor++
		}
	case "enter":
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

func (m RuntimeDashboard) ReconfigureRequested() bool {
	return m.reconfigureRequested
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
	switch msg.String() {
	case "up", "k":
		m.settingsCursor = settingsCursorUp(m.settingsCursor)
	case "down", "j":
		m.settingsCursor = settingsCursorDown(m.settingsCursor, settingsVisibleRowCount(m.preferences, m.serverSupported))
	case "left", "h":
		prevTheme := m.preferences.Theme
		m.preferences = applySettingsChange(m.settings, m.settingsCursor, -1, m.serverSupported)
		if m.settingsCursor == settingsThemeRow && m.preferences.Theme != prevTheme {
			cmd = tea.ClearScreen
		}
	case "right", "l", "enter":
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
	if m.mode == config.ModeServer {
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
	if protocol := m.protocolLine(); protocol != "" {
		body = append(body, protocol)
	}
	if serverLines := m.serverAddressLines(); len(serverLines) > 0 {
		body = append(body, serverLines...)
	}
	if tunnelLines := m.tunnelIPLines(); len(tunnelLines) > 0 {
		body = append(body, tunnelLines...)
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
		body = append(body, "", m.stopConfirmTitle(), "")
		body = append(body, renderSelectableRows(
			[]string{"Continue", m.stopActionLabel()},
			m.confirmCursor,
			contentWidth,
			styles,
		)...)
	}
	hint := m.dataplaneHint()

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

func (m RuntimeDashboard) serverAddressLines() []string {
	if len(m.endpoints) == 0 {
		return nil
	}
	if m.mode == config.ModeServer && len(m.endpoints) > 1 {
		if sharedAddress, ok := sharedServerAddress(m.endpoints); ok {
			return []string{formatRuntimeHostLine("Server IP", sharedAddress)}
		}
		lines := []string{"Server IPs:"}
		for _, endpoint := range m.endpoints {
			if line := formatRuntimeProtocolHost(endpoint.Protocol, endpoint.Server); line != "" {
				lines = append(lines, "  "+line)
			}
		}
		if len(lines) > 1 {
			return lines
		}
		return nil
	}
	if line := formatRuntimeHostLine("Server IP", m.endpoints[0].Server); line != "" {
		return []string{line}
	}
	return nil
}

func (m RuntimeDashboard) tunnelIPLines() []string {
	if len(m.endpoints) == 0 {
		return nil
	}
	if m.mode == config.ModeServer && len(m.endpoints) > 1 {
		lines := []string{"Tunnel IPs:"}
		for _, endpoint := range m.endpoints {
			if line := formatRuntimeProtocolAddress(endpoint.Protocol, endpoint.TunnelIPv4, endpoint.TunnelIPv6); line != "" {
				lines = append(lines, "  "+line)
			}
		}
		if len(lines) > 1 {
			return lines
		}
	}
	if tunnelIP := formatRuntimeAddressLine("Tunnel IP", m.endpoints[0].TunnelIPv4, m.endpoints[0].TunnelIPv6); tunnelIP != "" {
		return []string{tunnelIP}
	}
	return nil
}

func (m RuntimeDashboard) protocolLine() string {
	if m.mode != config.ModeClient || m.protocol == settings.UNKNOWN {
		return ""
	}
	return "Protocol: " + m.protocol.String()
}

func formatRuntimeHostLine(label string, host settings.Host) string {
	parts := formatRuntimeHostParts(host)
	if parts == "" {
		return ""
	}
	return label + ": " + parts
}

func formatRuntimeHostParts(host settings.Host) string {
	parts := make([]string, 0, 3)
	if host.Domain != "" {
		parts = append(parts, "Domain "+host.Domain)
	}
	if host.IPv4 != "" {
		parts = append(parts, "IPv4 "+host.IPv4)
	}
	if host.IPv6 != "" {
		parts = append(parts, "IPv6 "+host.IPv6)
	}
	return strings.Join(parts, " | ")
}

func formatRuntimeAddressLine(label string, ipv4, ipv6 netip.Addr) string {
	parts := formatRuntimeAddressParts(ipv4, ipv6)
	if parts == "" {
		return ""
	}
	return label + ": " + parts
}

func formatRuntimeAddressParts(ipv4, ipv6 netip.Addr) string {
	parts := make([]string, 0, 2)
	if ipv4.IsValid() {
		parts = append(parts, "IPv4 "+ipv4.String())
	}
	if ipv6.IsValid() {
		parts = append(parts, "IPv6 "+ipv6.String())
	}
	return strings.Join(parts, " | ")
}

func formatRuntimeProtocolHost(protocol settings.Protocol, host settings.Host) string {
	parts := formatRuntimeHostParts(host)
	if parts == "" {
		return ""
	}
	if protocol == settings.UNKNOWN {
		return parts
	}
	return protocol.String() + ": " + parts
}

func formatRuntimeProtocolAddress(protocol settings.Protocol, ipv4, ipv6 netip.Addr) string {
	parts := formatRuntimeAddressParts(ipv4, ipv6)
	if parts == "" {
		return ""
	}
	if protocol == settings.UNKNOWN {
		return parts
	}
	return protocol.String() + ": " + parts
}

func sharedServerAddress(endpoints []config.EndpointInfo) (settings.Host, bool) {
	if len(endpoints) == 0 {
		return settings.Host{}, false
	}
	sharedAddress := endpoints[0].Server
	if sharedAddress == (settings.Host{}) {
		return settings.Host{}, false
	}
	for _, endpoint := range endpoints[1:] {
		if endpoint.Server != sharedAddress {
			return settings.Host{}, false
		}
	}
	return sharedAddress, true
}

func (m RuntimeDashboard) stopActionLabel() string {
	if m.mode == config.ModeClient && m.preferences.AutoConnect {
		return "Stop (AutoConnect will be disabled)"
	}
	return "Stop"
}

func (m RuntimeDashboard) stopConfirmTitle() string {
	if m.mode == config.ModeServer {
		return runtimeStopConfirmTitleServer
	}
	return runtimeStopConfirmTitleClient
}

func (m RuntimeDashboard) dataplaneHint() string {
	if m.confirmOpen {
		return runtimeHintDataplaneConfirmOpen
	}
	if m.mode == config.ModeClient && !m.connected {
		return runtimeHintDataplaneReconfigure
	}
	return runtimeHintDataplaneStopConfirm
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
	return renderTabsLine(productLabel(), runtimeTabs[:], int(m.screen), contentWidth, styles)
}

func (m RuntimeDashboard) logsHint() string {
	return "up/down scroll | PgUp/PgDn page | Home/End jump | Space follow | Tab switch tabs | Esc back | ctrl+c exit"
}

func (m RuntimeDashboard) updateLogs(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, m.logs.updateKeys(msg)
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
) tea.Cmd {
	changeFeed, ok := feed.(RuntimeLogChangeFeed)
	if ok {
		changes := changeFeed.Changes()
		if changes != nil {
			return func() tea.Msg {
				select {
				case <-ctx.Done():
					return runtimeContextDoneMsg{}
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

func waitForRuntimeContextDone(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return runtimeContextDoneMsg{}
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
