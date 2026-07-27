package bubble_tea

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"tungo/application/runtime"
	"tungo/infrastructure/PAL/service_management/linux/systemd"

	tea "charm.land/bubbletea/v2"
)

func (m configuratorSessionModel) updateDaemonManageScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.leaveDaemonManageScreen(), nil
	}

	m.updateCursor(msg, len(m.daemon.menuOptions))
	if msg.String() != "enter" || len(m.daemon.menuOptions) == 0 {
		return m, nil
	}

	selected := m.daemon.menuOptions[m.cursor]
	selectedCursor := m.cursor
	var err error
	switch selected {
	case sessionDaemonSetupClient:
		m, err = m.applyDaemonSetup(runtime.ModeClient, false)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
	case sessionDaemonSetupServer:
		m, err = m.applyDaemonSetup(runtime.ModeServer, false)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
	case sessionDaemonReconfClient:
		if daemonStateBlocksRuntimeStart(string(m.daemon.status.ActiveState)) {
			m.pendingDaemonMode = runtime.ModeClient
			m.cursor = 0
			m.screen = configuratorScreenDaemonReconfigureConfirm
			return m, nil
		}
		m, err = m.applyDaemonSetup(runtime.ModeClient, false)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
	case sessionDaemonReconfServer:
		if daemonStateBlocksRuntimeStart(string(m.daemon.status.ActiveState)) {
			m.pendingDaemonMode = runtime.ModeServer
			m.cursor = 0
			m.screen = configuratorScreenDaemonReconfigureConfirm
			return m, nil
		}
		m, err = m.applyDaemonSetup(runtime.ModeServer, false)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
	case sessionDaemonStart:
		if m.options.Daemon == nil {
			m.notice = "Daemon start is unavailable."
			return m, nil
		}
		if err := m.options.Daemon.StartUnit(); err != nil {
			m.notice = fmt.Sprintf("Failed to start daemon: %v", err)
			return m, nil
		}
		m.notice = ""
	case sessionDaemonStop:
		if m.options.Daemon == nil {
			m.notice = "Daemon stop is unavailable."
			return m, nil
		}
		if err := m.options.Daemon.StopUnit(); err != nil {
			m.notice = fmt.Sprintf("Failed to stop daemon: %v", err)
			return m, nil
		}
		m.notice = ""
	case sessionDaemonEnable:
		if m.options.Daemon == nil {
			m.notice = "Daemon enable is unavailable."
			return m, nil
		}
		if err := m.options.Daemon.EnableUnit(); err != nil {
			m.notice = fmt.Sprintf("Failed to enable daemon: %v", err)
			return m, nil
		}
		m.notice = ""
	case sessionDaemonDisable:
		if m.options.Daemon == nil {
			m.notice = "Daemon disable is unavailable."
			return m, nil
		}
		if err := m.options.Daemon.DisableUnit(); err != nil {
			m.notice = fmt.Sprintf("Failed to disable daemon: %v", err)
			return m, nil
		}
		m.notice = ""
	case sessionDaemonDelete:
		if m.options.Daemon == nil {
			m.notice = "Daemon remove is unavailable."
			return m, nil
		}
		if err := m.options.Daemon.RemoveUnit(); err != nil {
			m.notice = fmt.Sprintf("Failed to remove daemon: %v", err)
			return m, nil
		}
		m.notice = ""
	default:
		return m, nil
	}

	m.refreshDaemonStatus()
	m.cursor = daemonMenuCursorAfterRefresh(m.daemon.menuOptions, selected, selectedCursor)
	return m, nil
}

func (m configuratorSessionModel) updateDaemonReconfigureConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = configuratorScreenDaemonManage
		m.cursor = 0
		m.pendingDaemonMode = 0
		m.notice = "Reconfigure cancelled."
		return m, nil
	}

	options := []string{sessionDaemonConfirmReconfigureNow, sessionCancel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}

	if options[m.cursor] == sessionCancel {
		m.screen = configuratorScreenDaemonManage
		m.cursor = 0
		m.pendingDaemonMode = 0
		m.notice = "Reconfigure cancelled."
		return m, nil
	}

	targetMode := m.pendingDaemonMode
	m.pendingDaemonMode = 0
	m.screen = configuratorScreenDaemonManage
	m.cursor = 0

	updated, err := m.applyDaemonSetup(targetMode, true)
	if err != nil {
		updated.notice = err.Error()
		return updated, nil
	}
	return updated, nil
}

func (m configuratorSessionModel) applyDaemonSetup(targetMode runtime.Mode, restartRunning bool) (configuratorSessionModel, error) {
	if m.options.Daemon == nil {
		return m, errors.New("daemon setup is unavailable")
	}
	if targetMode == runtime.ModeClient {
		if err := m.options.ClientConfigurationControl.ValidateActive(); err != nil {
			return m, fmt.Errorf("cannot setup client daemon: %v", err)
		}
	}

	path, err := m.options.Daemon.Setup(targetMode)
	if err != nil {
		return m, fmt.Errorf("failed to setup daemon: %v", err)
	}

	if restartRunning {
		role := "Client"
		if targetMode == runtime.ModeServer {
			role = "Server"
		}
		m.notice = fmt.Sprintf("%s daemon reconfigured at %s and restarted.", role, path)
	} else {
		m.notice = ""
	}
	m.refreshDaemonStatus()
	m.cursor = 0
	return m, nil
}

func (m configuratorSessionModel) updateDaemonActiveConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m = m.cancelPendingDaemonStart("Start cancelled.")
		return m, nil
	}

	options := []string{sessionStopDaemonContinue, sessionCancel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == sessionCancel {
		m = m.cancelPendingDaemonStart("Start cancelled.")
		return m, nil
	}

	if m.options.Daemon == nil {
		m = m.cancelPendingDaemonStart("Stopping daemon is unavailable.")
		return m, nil
	}

	if err := m.options.Daemon.StopUnit(); err != nil {
		m = m.cancelPendingDaemonStart(fmt.Sprintf("Failed to stop daemon: %v", err))
		return m, nil
	}

	return m.completePendingDaemonStart("Daemon stopped. Starting selected mode.")
}

func (m configuratorSessionModel) updateDaemonCheckErrorConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m = m.cancelPendingDaemonStart("Start cancelled.")
		return m, nil
	}

	options := []string{sessionRetryDaemonCheck, sessionStartAnywayUnsafe, sessionCancel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}

	selected := options[m.cursor]
	switch selected {
	case sessionCancel:
		m = m.cancelPendingDaemonStart("Start cancelled.")
		return m, nil
	case sessionStartAnywayUnsafe:
		return m.completePendingDaemonStart("Daemon status check failed. Starting selected mode without daemon guard.")
	case sessionRetryDaemonCheck:
		targetMode := m.pendingStartMode
		returnScreen := m.pendingStartScreen
		pendingClientConfig := m.pendingClientConfig
		m = m.startModeWithDaemonGuard(targetMode, returnScreen, true)
		if m.done {
			return m, tea.Quit
		}
		if !m.done && isDaemonStartConfirmationScreen(m.screen) && targetMode == runtime.ModeClient {
			m.pendingClientConfig = pendingClientConfig
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m configuratorSessionModel) completePendingDaemonStart(notice string) (configuratorSessionModel, tea.Cmd) {
	targetMode := m.pendingStartMode
	pendingClientConfig := m.pendingClientConfig
	m = m.clearPendingDaemonStart()
	if targetMode == runtime.ModeClient {
		m = m.persistAutoSelectClientConfig(pendingClientConfig)
	}
	m.notice = notice
	m.resultMode = targetMode
	m.done = true
	return m, tea.Quit
}

func (m configuratorSessionModel) startModeWithDaemonGuard(targetMode runtime.Mode, returnScreen configuratorScreen, preserveNotice bool) configuratorSessionModel {
	m = m.clearPendingDaemonStart()

	if m.options.Daemon == nil {
		m.resultMode = targetMode
		m.done = true
		return m
	}

	active, err := m.options.Daemon.IsUnitActive()
	if err != nil {
		message := fmt.Sprintf("Failed to check daemon status: %v", err)
		if preserveNotice {
			m.notice = appendNotice(m.notice, message)
		} else {
			m.notice = message
		}
		m.cursor = 0
		m.pendingStartMode = targetMode
		m.pendingStartScreen = returnScreen
		m.screen = configuratorScreenDaemonCheckErrorConfirm
		return m
	}
	if !active {
		m.resultMode = targetMode
		m.done = true
		return m
	}
	if !preserveNotice {
		m.notice = ""
	}
	m.cursor = 0
	m.pendingStartMode = targetMode
	m.pendingStartScreen = returnScreen
	m.screen = configuratorScreenDaemonActiveConfirm
	return m
}

func (m configuratorSessionModel) cancelPendingDaemonStart(notice string) configuratorSessionModel {
	returnScreen := m.pendingStartScreen
	m = m.clearPendingDaemonStart()
	m.notice = notice
	m.cursor = 0
	m.screen = returnScreen
	return m
}

func (m configuratorSessionModel) clearPendingDaemonStart() configuratorSessionModel {
	m.pendingStartMode = 0
	m.pendingStartScreen = configuratorScreenMode
	m.pendingClientConfig = ""
	return m
}

func isDaemonStartConfirmationScreen(screen configuratorScreen) bool {
	switch screen {
	case configuratorScreenDaemonActiveConfirm, configuratorScreenDaemonCheckErrorConfirm:
		return true
	default:
		return false
	}
}

func (m *configuratorSessionModel) refreshDaemonStatus() {
	if m.options.Daemon == nil {
		m.daemon.statusErr = errors.New("daemon management is unavailable")
		m.daemon.status = systemd.UnitStatus{}
		m.daemon.menuOptions = nil
		m.daemon.updatedAt = time.Time{}
		return
	}

	status, err := m.options.Daemon.Status()
	if err != nil {
		m.daemon.statusErr = err
		m.daemon.status = systemd.UnitStatus{}
		m.daemon.menuOptions = nil
		m.daemon.updatedAt = time.Time{}
		return
	}
	m.daemon.statusErr = nil
	m.daemon.status = status
	m.daemon.menuOptions = m.daemonMenuOptions(status)
	m.daemon.updatedAt = time.Now()
}

func (m configuratorSessionModel) daemonMenuOptions(status systemd.UnitStatus) []string {
	if m.options.Daemon == nil {
		return nil
	}
	options := make([]string, 0, 7)
	if !status.Installed {
		options = append(options, sessionDaemonSetupClient)
		if m.serverSupported {
			options = append(options, sessionDaemonSetupServer)
		}
		return options
	}

	activeBlocksStart := daemonStateBlocksRuntimeStart(string(status.ActiveState))
	if activeBlocksStart {
		options = append(options, sessionDaemonStop)
	}
	if !activeBlocksStart && daemonStateAllowsStart(string(status.ActiveState)) {
		options = append(options, sessionDaemonStart)
	}
	switch normalizeDaemonStateField(string(status.UnitFileState)) {
	case "enabled":
		options = append(options, sessionDaemonDisable)
	case "disabled":
		options = append(options, sessionDaemonEnable)
	}
	options = append(options, sessionDaemonReconfClient)
	if m.serverSupported {
		options = append(options, sessionDaemonReconfServer)
	}
	if status.Managed {
		options = append(options, sessionDaemonDelete)
	}
	return options
}

func (m configuratorSessionModel) daemonNotice() string {
	statusLine := m.daemonStatusLine()
	notice := strings.TrimSpace(m.notice)
	if notice == "" {
		return statusLine
	}
	return statusLine + "\n" + notice
}

func (m configuratorSessionModel) daemonStatusLine() string {
	if m.daemon.statusErr != nil {
		return "Status error: " + m.daemon.statusErr.Error()
	}
	loadState := normalizeDaemonStateField(string(m.daemon.status.LoadState))
	unitFileState := normalizeDaemonStateField(string(m.daemon.status.UnitFileState))
	activeState := normalizeDaemonStateField(string(m.daemon.status.ActiveState))
	subState := normalizeDaemonStateField(m.daemon.status.SubState)
	result := normalizeDaemonStateField(m.daemon.status.Result)
	execMainStatus := normalizeDaemonStateField(m.daemon.status.ExecMainStatus)
	execStart := normalizeDaemonRawField(m.daemon.status.ExecStart)
	fragmentPath := normalizeDaemonRawField(m.daemon.status.FragmentPath)
	derivedRole, derivedRoleSource := daemonDerivedRole(m.daemon.status, execStart)
	return strings.Join([]string{
		fmt.Sprintf("Active: %s", activeState),
		fmt.Sprintf("Sub: %s", subState),
		fmt.Sprintf("Result: %s", result),
		fmt.Sprintf("UnitFile: %s", unitFileState),
		fmt.Sprintf("Load: %s", loadState),
		fmt.Sprintf("ExecMainStatus: %s", execMainStatus),
		fmt.Sprintf("ExecStart: %s", execStart),
		fmt.Sprintf("FragmentPath: %s", fragmentPath),
		fmt.Sprintf("DerivedRole: %s (from %s)", derivedRole, derivedRoleSource),
	}, "\n")
}

func normalizeDaemonStateField(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func normalizeDaemonRawField(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func daemonRoleFromExecStart(execStart string) string {
	raw := strings.ToLower(strings.TrimSpace(execStart))
	if raw == "" || raw == "unknown" {
		return "unknown"
	}
	if strings.Contains(raw, " tungo c") || strings.Contains(raw, "/tungo c") {
		return "client"
	}
	if strings.Contains(raw, " tungo s") || strings.Contains(raw, "/tungo s") {
		return "server"
	}
	return "unknown"
}

func daemonDerivedRole(status systemd.UnitStatus, execStart string) (string, string) {
	if role := daemonRoleFromExecStart(execStart); role != "unknown" {
		return role, "ExecStart"
	}
	switch status.Role {
	case systemd.UnitRoleClient:
		return "client", "Role"
	case systemd.UnitRoleServer:
		return "server", "Role"
	default:
		return "unknown", "Role"
	}
}

func daemonStateBlocksRuntimeStart(activeState string) bool {
	switch normalizeDaemonStateField(activeState) {
	case "active", "reloading", "activating", "deactivating":
		return true
	default:
		return false
	}
}

func daemonStateAllowsStart(activeState string) bool {
	switch normalizeDaemonStateField(activeState) {
	case "inactive", "failed":
		return true
	default:
		return false
	}
}

func (m configuratorSessionModel) leaveDaemonManageScreen() configuratorSessionModel {
	m.tab = configuratorTabMain
	m.screen = configuratorScreenMode
	if idx := slices.Index(m.modeOptions, sessionModeDaemon); idx >= 0 {
		m.cursor = idx
	} else {
		m.cursor = 0
	}
	m.pendingDaemonMode = 0
	m.refreshDaemonStatus()
	return m
}

func (m configuratorSessionModel) renderDaemonManageScreen() string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	rows := renderSelectableRows(m.daemon.menuOptions, m.cursor, contentWidth, styles)
	body := make([]string, 0, len(rows)+18)
	body = append(body, styles.title.Render("Daemon Status"))
	body = append(body, daemonSectionDivider(contentWidth))
	body = append(body, strings.Split(m.daemonStatusLine(), "\n")...)
	if !m.daemon.updatedAt.IsZero() {
		body = append(body, styles.meta.Render("Updated: "+m.daemon.updatedAt.Format("15:04:05")))
	}

	if notice := strings.TrimSpace(m.notice); notice != "" {
		body = append(body, "", notice)
	}

	body = append(body, "", styles.title.Render("Actions"), daemonSectionDivider(contentWidth))
	body = append(body, rows...)

	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"Setup/Manage daemon",
		body,
		"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func daemonSectionDivider(contentWidth int) string {
	if contentWidth <= 0 {
		return strings.Repeat("-", 24)
	}
	return strings.Repeat("-", maxInt(12, minInt(40, contentWidth)))
}

func daemonMenuCursorAfterRefresh(options []string, selected string, fallbackCursor int) int {
	if len(options) == 0 {
		return 0
	}
	if selected = strings.TrimSpace(selected); selected != "" {
		if idx := slices.Index(options, selected); idx >= 0 {
			return idx
		}
	}
	if fallbackCursor < 0 {
		return 0
	}
	return minInt(fallbackCursor, len(options)-1)
}
