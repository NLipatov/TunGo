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
type daemonUnitStatus struct {
	status systemd.UnitStatus
	err    error
}

type daemonStatusMsg struct {
	units  map[mode.Mode]daemonUnitStatus
	notice string
}

// Each menu entry carries its target, so labels never determine service ownership.
type daemonMenuOption struct {
	mode   mode.Mode
	action string
}

func (o daemonMenuOption) String() string {
	if o.action == daemonSetupClientLabel || o.action == daemonSetupServerLabel {
		return o.action
	}
	role := "client"
	if o.mode == mode.Server {
		role = "server"
	}
	return role + ": " + o.action
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
	switch selected.action {
	case daemonSetupClientLabel, daemonSetupServerLabel:
		return m.beginDaemonOperation("Installing daemon...", func() error { return m.applyDaemonSetup(selected.mode) })
	}
	var action func(context.Context, mode.Mode) error
	var verb, progress string
	switch selected.action {
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
	return m.beginDaemonOperation(progress, func() error {
		if err := action(ctx, selected.mode); err != nil {
			return fmt.Errorf("Failed to %s daemon: %w", verb, err)
		}
		return nil
	})
}

func (m Configurator) applyDaemonSetup(targetMode mode.Mode) error {
	if m.options.Daemon == nil {
		return errors.New("daemon setup is unavailable")
	}
	if targetMode == mode.Client {
		if _, err := m.options.ClientConfigurations.Active(); err != nil {
			return fmt.Errorf("cannot setup client daemon: %w", err)
		}
	}
	_, err := m.options.Daemon.Setup(m.daemon.commands.ctx, targetMode)
	if err != nil {
		return fmt.Errorf("failed to setup daemon: %w", err)
	}

	return nil
}

func (m Configurator) beginDaemonOperation(progress string, action func() error) (Configurator, tea.Cmd) {
	if m.daemon.pending != "" {
		return m, nil
	}
	m.daemon.pending = progress
	daemon, ctx := m.options.Daemon, m.daemon.commands.ctx
	return m, m.daemon.commands.track(func() tea.Msg {
		result := daemonStatusMsg{units: make(map[mode.Mode]daemonUnitStatus)}
		if action != nil {
			if err := action(); err != nil {
				result.notice = err.Error()
				if errors.Is(err, context.DeadlineExceeded) {
					result.notice = "Daemon operation timed out; check the current status."
				}
			}
		}
		// Even a failed or timed-out command may have changed the service state.
		for _, runtimeMode := range []mode.Mode{mode.Client, mode.Server} {
			if runtimeMode == mode.Server && !m.serverSupported {
				continue
			}
			unit := daemonUnitStatus{}
			if daemon == nil {
				unit.err = errors.New("daemon management is unavailable")
			} else {
				unit.status, unit.err = daemon.Status(ctx, runtimeMode)
			}
			result.units[runtimeMode] = unit
		}
		return result
	})
}

func (m Configurator) refreshDaemonStatus() (Configurator, tea.Cmd) {
	return m.beginDaemonOperation("Checking daemon status...", nil)
}

func (m Configurator) applyDaemonStatus(msg daemonStatusMsg) Configurator {
	m.daemon.pending = ""
	m.daemon.units = msg.units
	selected := daemonMenuOption{}
	if m.screen == configuratorScreenDaemonManage && m.cursor < len(m.daemon.menuOptions) {
		selected = m.daemon.menuOptions[m.cursor]
	}
	m.daemon.menuOptions = nil
	for _, runtimeMode := range []mode.Mode{mode.Client, mode.Server} {
		unit, ok := msg.units[runtimeMode]
		if !ok || unit.err != nil {
			continue
		}
		for _, action := range m.daemonMenuOptions(unit.status, runtimeMode) {
			m.daemon.menuOptions = append(m.daemon.menuOptions, daemonMenuOption{mode: runtimeMode, action: action})
		}
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
	return m, m.daemon.commands.track(func() tea.Msg { return daemonStoppedMsg{err: daemon.StopUnit(ctx, m.pendingStartMode)} })
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
		active, err := daemon.IsUnitActive(ctx, targetMode)
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

func (m Configurator) daemonMenuOptions(status systemd.UnitStatus, runtimeMode mode.Mode) []string {
	if m.options.Daemon == nil {
		return nil
	}
	options := make([]string, 0, 7)
	if !status.Installed {
		if runtimeMode == mode.Server {
			return []string{daemonSetupServerLabel}
		}
		return []string{daemonSetupClientLabel}
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

	if status.Managed {
		options = append(options, daemonDeleteLabel)
	}
	return options
}

func daemonStatusLine(unit daemonUnitStatus) string {
	if unit.err != nil {
		if errors.Is(unit.err, context.DeadlineExceeded) {
			return "Status check timed out."
		}
		return "Status error: " + strings.Join(strings.Fields(unit.err.Error()), " ")
	}
	if !unit.status.Installed {
		return "not installed"
	}
	activeState := normalizeDaemonStateField(string(unit.status.ActiveState))
	subState := normalizeDaemonStateField(unit.status.SubState)
	if activeState == "inactive" {
		activeState = "stopped"
	} else if subState != "unknown" && subState != "dead" {
		activeState += " / " + subState
	}
	return fmt.Sprintf("%s | Autostart: %s", activeState, normalizeDaemonStateField(string(unit.status.UnitFileState)))
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
	m.notice = ""
	return m
}

func (m Configurator) renderDaemonManageScreen() string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	labels := make([]string, len(m.daemon.menuOptions))
	for idx, option := range m.daemon.menuOptions {
		labels[idx] = option.String()
	}
	rows := renderSelectableRows(labels, m.cursor, contentWidth, styles)
	body := make([]string, 0, len(rows)+4)
	// Bound status and notices so they cannot push actions out of a short terminal.
	for _, runtimeMode := range []mode.Mode{mode.Client, mode.Server} {
		if runtimeMode == mode.Server && !m.serverSupported {
			continue
		}
		role := "Client"
		if runtimeMode == mode.Server {
			role = "Server"
		}
		line := role + ": checking..."
		if unit, ok := m.daemon.units[runtimeMode]; ok {
			line = role + ": " + daemonStatusLine(unit)
		}
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
		"Setup/Manage daemons",
		body,
		"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func daemonMenuCursorAfterRefresh(options []daemonMenuOption, selected daemonMenuOption, fallbackCursor int) int {
	if len(options) == 0 {
		return 0
	}
	if selected.action != "" {
		if idx := slices.Index(options, selected); idx >= 0 {
			return idx
		}
		// A removed action must not move the cursor onto the other service.
		if fallbackCursor >= 0 && fallbackCursor < len(options) && options[fallbackCursor].mode == selected.mode {
			return fallbackCursor
		}
		if idx := slices.IndexFunc(options, func(option daemonMenuOption) bool { return option.mode == selected.mode }); idx >= 0 {
			return idx
		}
	}
	if fallbackCursor < 0 {
		return 0
	}
	return minInt(fallbackCursor, len(options)-1)
}
