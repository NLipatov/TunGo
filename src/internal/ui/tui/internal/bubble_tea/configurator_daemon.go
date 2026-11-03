package bubble_tea

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"tungo/internal/daemon/systemd"
	"tungo/internal/mode"
)

// Only the event loop changes the model. Commands return data, never a model snapshot.
type daemonStatusMsg struct {
	status systemd.UnitStatus
	err    error
	notice string
}

type daemonActiveMsg struct {
	active bool
	err    error
}

type daemonStoppedMsg struct{ err error }

// daemonCommands owns the shared lifetime of commands across model copies.
type daemonCommands struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	active sync.WaitGroup
}

func (c *daemonCommands) track(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		if c.ctx.Err() != nil {
			c.mu.Unlock()
			return nil
		}
		c.active.Add(1)
		c.mu.Unlock()
		defer c.active.Done()
		return cmd()
	}
}

func (c *daemonCommands) close() {
	// Serialize cancellation with registration: a command that Bubble Tea starts
	// late must not begin work after Wait has observed zero active commands.
	c.mu.Lock()
	c.cancel()
	c.mu.Unlock()
	c.active.Wait()
}

func (m Configurator) updateDaemonManageScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m.leaveDaemonManageScreen(), nil
	}
	m.updateCursor(msg, len(m.daemon.menuOptions))
	if msg.String() != "enter" || len(m.daemon.menuOptions) == 0 || m.daemon.pending != "" {
		return m, nil
	}
	selected := m.daemon.menuOptions[m.cursor]
	switch selected {
	case daemonSetupClientLabel:
		return m.setupDaemon(mode.Client, false)
	case daemonSetupServerLabel:
		return m.setupDaemon(mode.Server, false)
	case daemonReconfigureClientLabel, daemonReconfigureServerLabel:
		target := mode.Client
		if selected == daemonReconfigureServerLabel {
			target = mode.Server
		}
		if daemonStateBlocksRuntimeStart(string(m.daemon.status.ActiveState)) {
			m.pendingDaemonMode = target
			m.cursor = 0
			m.screen = configuratorScreenDaemonReconfigureConfirm
			return m, nil
		}
		return m.setupDaemon(target, false)
	}
	var action func(context.Context) error
	var verb, progress string
	switch selected {
	case daemonStartLabel:
		verb, progress = "start", "Starting daemon..."
		if m.options.Daemon != nil {
			action = m.options.Daemon.StartUnit
		}
	case daemonStopLabel:
		verb, progress = "stop", "Stopping daemon..."
		if m.options.Daemon != nil {
			action = m.options.Daemon.StopUnit
		}
	case daemonEnableLabel:
		verb, progress = "enable", "Enabling autostart..."
		if m.options.Daemon != nil {
			action = m.options.Daemon.EnableUnit
		}
	case daemonDisableLabel:
		verb, progress = "disable", "Disabling autostart..."
		if m.options.Daemon != nil {
			action = m.options.Daemon.DisableUnit
		}
	case daemonDeleteLabel:
		verb, progress = "remove", "Removing daemon..."
		if m.options.Daemon != nil {
			action = m.options.Daemon.RemoveUnit
		}
	default:
		return m, nil
	}
	if action == nil {
		m.notice = fmt.Sprintf("Daemon %s is unavailable.", verb)
		return m, nil
	}
	ctx := m.daemon.commands.ctx
	return m.beginDaemonOperation(progress, func() (string, error) {
		if err := action(ctx); err != nil {
			return "", fmt.Errorf("Failed to %s daemon: %w", verb, err)
		}
		return "", nil
	})
}

func (m Configurator) updateDaemonReconfigureConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = configuratorScreenDaemonManage
		m.cursor = 0
		m.pendingDaemonMode = 0
		m.notice = "Reconfigure cancelled."
		return m, nil
	}
	options := []string{daemonConfirmReconfigureNowLabel, cancelLabel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}
	if options[m.cursor] == cancelLabel {
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
	return m.setupDaemon(targetMode, true)
}

func (m Configurator) setupDaemon(targetMode mode.Mode, restartRunning bool) (Configurator, tea.Cmd) {
	return m.beginDaemonOperation("Configuring daemon...", func() (string, error) {
		return m.applyDaemonSetup(targetMode, restartRunning)
	})
}

func (m Configurator) applyDaemonSetup(targetMode mode.Mode, restartRunning bool) (string, error) {
	if m.options.Daemon == nil {
		return "", errors.New("daemon setup is unavailable")
	}
	if targetMode == mode.Client {
		if _, err := m.options.ClientConfigurations.Active(); err != nil {
			return "", fmt.Errorf("cannot setup client daemon: %w", err)
		}
	}
	path, err := m.options.Daemon.Setup(m.daemon.commands.ctx, targetMode)
	if err != nil {
		return "", fmt.Errorf("failed to setup daemon: %w", err)
	}
	if restartRunning {
		role := "Client"
		if targetMode == mode.Server {
			role = "Server"
		}
		return fmt.Sprintf("%s daemon reconfigured at %s and restarted.", role, path), nil
	}
	return "", nil
}

func (m Configurator) beginDaemonOperation(progress string, action func() (string, error)) (Configurator, tea.Cmd) {
	if m.daemon.pending != "" {
		return m, nil
	}
	m.daemon.pending = progress
	daemon, ctx := m.options.Daemon, m.daemon.commands.ctx
	return m, m.daemon.commands.track(func() tea.Msg {
		result := daemonStatusMsg{}
		if action != nil {
			notice, err := action()
			result.notice = notice
			if err != nil {
				result.notice = err.Error()
				if errors.Is(err, context.DeadlineExceeded) {
					result.notice = "Daemon operation timed out; check the current status."
				}
			}
		}
		// Even a failed or timed-out command may have changed the service state.
		if daemon == nil {
			result.err = errors.New("daemon management is unavailable")
		} else {
			result.status, result.err = daemon.Status(ctx)
		}
		return result
	})
}

func (m Configurator) refreshDaemonStatus() (Configurator, tea.Cmd) {
	return m.beginDaemonOperation("Checking daemon status...", nil)
}

func (m Configurator) applyDaemonStatus(msg daemonStatusMsg) Configurator {
	m.daemon.pending = ""
	m.daemon.status, m.daemon.statusErr = msg.status, msg.err
	selected := ""
	if m.screen == configuratorScreenDaemonManage && m.cursor < len(m.daemon.menuOptions) {
		selected = m.daemon.menuOptions[m.cursor]
	}
	if msg.err != nil {
		m.daemon.menuOptions = nil
	} else {
		m.daemon.menuOptions = m.daemonMenuOptions(msg.status)
	}
	if m.screen == configuratorScreenDaemonManage {
		m.notice = msg.notice
		m.cursor = daemonMenuCursorAfterRefresh(m.daemon.menuOptions, selected, m.cursor)
	}
	return m
}

func (m Configurator) updateDaemonActiveConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m.cancelPendingDaemonStart("Start cancelled."), nil
	}
	options := []string{stopDaemonContinueLabel, cancelLabel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}
	if options[m.cursor] == cancelLabel {
		return m.cancelPendingDaemonStart("Start cancelled."), nil
	}
	if m.daemon.pending != "" {
		return m, nil
	}
	if m.options.Daemon == nil {
		return m.cancelPendingDaemonStart("Stopping daemon is unavailable."), nil
	}
	m.daemon.pending = "Stopping daemon..."
	m.notice = m.daemon.pending
	daemon, ctx := m.options.Daemon, m.daemon.commands.ctx
	return m, m.daemon.commands.track(func() tea.Msg { return daemonStoppedMsg{err: daemon.StopUnit(ctx)} })
}

func (m Configurator) updateDaemonCheckErrorConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m.cancelPendingDaemonStart("Start cancelled."), nil
	}
	options := []string{retryDaemonCheckLabel, startAnywayUnsafeLabel, cancelLabel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}
	switch options[m.cursor] {
	case cancelLabel:
		return m.cancelPendingDaemonStart("Start cancelled."), nil
	case startAnywayUnsafeLabel:
		return m.completePendingDaemonStart("Daemon status check failed. Starting selected mode without daemon guard.")
	case retryDaemonCheckLabel:
		return m.startModeWithDaemonGuard(m.pendingStartMode, m.pendingStartScreen, true)
	default:
		return m, nil
	}
}

func (m Configurator) completePendingDaemonStart(notice string) (Configurator, tea.Cmd) {
	targetMode := m.pendingStartMode
	pendingClientConfig := m.pendingClientConfig
	m = m.clearPendingDaemonStart()
	m.notice = notice
	if targetMode == mode.Client {
		m = m.persistAutoSelectClientConfig(pendingClientConfig)
	}
	m.resultMode = targetMode
	m.done = true
	return m, tea.Quit
}

func (m Configurator) startModeWithDaemonGuard(targetMode mode.Mode, returnScreen configuratorScreen, preserveNotice bool) (Configurator, tea.Cmd) {
	if m.daemon.pending != "" {
		m.notice = "Wait for the daemon operation to finish."
		return m, nil
	}
	m.pendingStartMode, m.pendingStartScreen = targetMode, returnScreen
	if m.options.Daemon == nil {
		return m.completePendingDaemonStart(m.notice)
	}
	if !preserveNotice {
		m.notice = ""
	}
	m.cursor = 0
	m.screen = configuratorScreenDaemonChecking
	m.daemon.pending = "Checking daemon status..."
	daemon, ctx := m.options.Daemon, m.daemon.commands.ctx
	return m, m.daemon.commands.track(func() tea.Msg {
		active, err := daemon.IsUnitActive(ctx)
		return daemonActiveMsg{active: active, err: err}
	})
}

func (m Configurator) applyDaemonActive(msg daemonActiveMsg) (Configurator, tea.Cmd) {
	m.daemon.pending = ""
	// Esc can withdraw the runtime start request while the check is in flight.
	if m.pendingStartMode == 0 {
		if m.screen == configuratorScreenDaemonManage {
			return m.refreshDaemonStatus()
		}
		return m, nil
	}
	if msg.err != nil {
		m.notice = appendNotice(m.notice, fmt.Sprintf("Failed to check daemon status: %v", msg.err))
		m.screen = configuratorScreenDaemonCheckErrorConfirm
		return m, nil
	}
	if !msg.active {
		return m.completePendingDaemonStart(m.notice)
	}
	m.screen = configuratorScreenDaemonActiveConfirm
	return m, nil
}

func (m Configurator) applyDaemonStopped(msg daemonStoppedMsg) (Configurator, tea.Cmd) {
	m.daemon.pending = ""
	if m.pendingStartMode == 0 {
		if m.screen == configuratorScreenDaemonManage {
			return m.refreshDaemonStatus()
		}
		return m, nil
	}
	if msg.err != nil {
		return m.cancelPendingDaemonStart(fmt.Sprintf("Failed to stop daemon: %v", msg.err)), nil
	}
	return m.completePendingDaemonStart("Daemon stopped. Starting selected mode.")
}

func (m Configurator) cancelPendingDaemonStart(notice string) Configurator {
	returnScreen := m.pendingStartScreen
	m = m.clearPendingDaemonStart()
	m.notice = notice
	m.cursor = 0
	m.screen = returnScreen
	return m
}

func (m Configurator) clearPendingDaemonStart() Configurator {
	m.pendingStartMode = 0
	m.pendingStartScreen = configuratorScreenMode
	m.pendingClientConfig = ""
	return m
}

func (m Configurator) daemonMenuOptions(status systemd.UnitStatus) []string {
	if m.options.Daemon == nil {
		return nil
	}
	options := make([]string, 0, 7)
	if !status.Installed {
		options = append(options, daemonSetupClientLabel)
		if m.serverSupported {
			options = append(options, daemonSetupServerLabel)
		}
		return options
	}

	activeBlocksStart := daemonStateBlocksRuntimeStart(string(status.ActiveState))
	if activeBlocksStart {
		options = append(options, daemonStopLabel)
	}
	if !activeBlocksStart && daemonStateAllowsStart(string(status.ActiveState)) {
		options = append(options, daemonStartLabel)
	}
	switch normalizeDaemonStateField(string(status.UnitFileState)) {
	case "enabled":
		options = append(options, daemonDisableLabel)
	case "disabled":
		options = append(options, daemonEnableLabel)
	}
	options = append(options, daemonReconfigureClientLabel)
	if m.serverSupported {
		options = append(options, daemonReconfigureServerLabel)
	}
	if status.Managed {
		options = append(options, daemonDeleteLabel)
	}
	return options
}

func (m Configurator) daemonStatusLine() string {
	if m.daemon.statusErr != nil {
		if errors.Is(m.daemon.statusErr, context.DeadlineExceeded) {
			return "Status check timed out."
		}
		return "Status error: " + strings.Join(strings.Fields(m.daemon.statusErr.Error()), " ")
	}
	if !m.daemon.status.Installed {
		return "State: not installed"
	}
	role := normalizeDaemonStateField(string(m.daemon.status.Role))
	unitFileState := normalizeDaemonStateField(string(m.daemon.status.UnitFileState))
	activeState := normalizeDaemonStateField(string(m.daemon.status.ActiveState))
	subState := normalizeDaemonStateField(m.daemon.status.SubState)
	if activeState == "inactive" {
		activeState = "stopped"
	} else if subState != "unknown" && subState != "dead" {
		activeState += " / " + subState
	}
	return fmt.Sprintf("Role: %s\nState: %s\nAutostart: %s", role, activeState, unitFileState)
}

func normalizeDaemonStateField(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "unknown"
	}
	return normalized
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

func (m Configurator) leaveDaemonManageScreen() Configurator {
	m.tab = configuratorTabMain
	m.screen = configuratorScreenMode
	if idx := slices.Index(m.modeOptions, modeDaemonLabel); idx >= 0 {
		m.cursor = idx
	} else {
		m.cursor = 0
	}
	m.pendingDaemonMode = 0
	m.notice = ""
	return m
}

func (m Configurator) renderDaemonManageScreen() string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	rows := renderSelectableRows(m.daemon.menuOptions, m.cursor, contentWidth, styles)
	body := make([]string, 0, len(rows)+4)
	// Bound status and notices so they cannot push actions out of a short terminal.
	for _, line := range strings.Split(m.daemonStatusLine(), "\n") {
		body = append(body, truncateWithEllipsis(line, contentWidth))
	}
	notice := m.notice
	if m.daemon.pending != "" {
		notice = m.daemon.pending
	}
	notice = strings.Join(strings.Fields(notice), " ")
	body = append(body, truncateWithEllipsis(notice, contentWidth))
	body = append(body, rows...)

	return renderScreenRaw(
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
