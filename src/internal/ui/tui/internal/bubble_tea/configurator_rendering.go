package bubble_tea

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"tungo/internal/config"
)

func (m Configurator) mainTabView() string {
	switch m.screen {
	case configuratorScreenMode:
		return m.renderSelectionScreen(
			"Select mode",
			m.notice,
			m.modeOptions,
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc exit | ctrl+c exit",
		)
	case configuratorScreenClientSelect:
		clientSelectHint := "up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit"
		if len(m.modeOptions) == 1 {
			clientSelectHint = "up/k down/j move | Enter select | Tab switch tabs | Esc exit | ctrl+c exit"
		}
		return m.renderSelectionScreen(
			"Select configuration - or add/remove one:",
			m.notice,
			m.client.menuOptions,
			m.cursor,
			clientSelectHint,
		)
	case configuratorScreenClientRemove:
		return m.renderSelectionScreen(
			"Choose a configuration to remove:",
			"",
			m.client.removePaths,
			m.cursor,
			"up/k down/j move | Enter remove | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenClientAddName:
		styles := resolveUIStyles(m.preferences)
		container := styles.inputFrame.Width(m.inputContainerWidth())
		stats := styles.meta.Render("Characters: " + formatCount(utf8.RuneCountInString(m.client.addNameInput.Value()), m.client.addNameInput.CharLimit))
		body := make([]string, 0, 4)
		if strings.TrimSpace(m.notice) != "" {
			body = append(body, m.notice, "")
		}
		body = append(body, container.Render(m.client.addNameInput.View()), stats)
		return renderScreen(
			m.width,
			m.height,
			m.tabsLine(styles),
			"Name configuration",
			body,
			"Enter confirm | Tab switch tabs | Esc back | ctrl+c exit",
			m.preferences,
			styles,
		)
	case configuratorScreenClientAddJSON:
		styles := resolveUIStyles(m.preferences)
		container := styles.inputFrame.Width(m.inputContainerWidth())
		lines := 1
		if value := m.client.addJSONInput.Value(); value != "" {
			lines = len(strings.Split(value, "\n"))
		}
		stats := styles.meta.Render(fmt.Sprintf("Lines: %d", lines))
		body := make([]string, 0, 4)
		if strings.TrimSpace(m.notice) != "" {
			body = append(body, m.notice, "")
		}
		body = append(body, container.Render(m.client.addJSONInput.View()), stats)
		return renderScreen(
			m.width,
			m.height,
			m.tabsLine(styles),
			"Paste configuration",
			body,
			"Enter confirm | Esc back | ctrl+c exit",
			m.preferences,
			styles,
		)
	case configuratorScreenClientInvalid:
		options := []string{invalidOKLabel}
		if m.client.invalidAllowDelete {
			options = []string{invalidDeleteLabel, invalidOKLabel}
		}
		subtitle := "Configuration is invalid: " + summarizeInvalidConfigurationError(m.client.invalidErr)
		return m.renderSelectionScreen(
			"Configuration error",
			subtitle,
			options,
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenServerSelect:
		return m.renderSelectionScreen(
			"Choose an option",
			m.notice,
			m.server.menuOptions,
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenServerManage:
		return m.renderSelectionScreen(
			"Select client to enable/disable or delete",
			"",
			m.server.manageLabels,
			m.cursor,
			"up/k down/j move | Enter toggle | d delete | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenServerDeleteConfirm:
		return m.renderSelectionScreen(
			fmt.Sprintf(
				"Delete client #%d %s?",
				m.server.deletePeer.ClientID,
				serverPeerDisplayName(m.server.deletePeer),
			),
			"This action removes client access from server configuration.",
			[]string{serverDeleteConfirmLabel, cancelLabel},
			m.cursor,
			"up/k down/j move | Enter confirm | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenDaemonManage:
		return m.renderDaemonManageScreen()
	case configuratorScreenDaemonReconfigureConfirm:
		roleLabel := "selected role"
		switch m.pendingDaemonMode {
		case config.ModeClient:
			roleLabel = "client"
		case config.ModeServer:
			roleLabel = "server"
		}
		return m.renderSelectionScreen(
			"Daemon is active",
			fmt.Sprintf("Applying %s daemon setup requires restart. Continue now?", roleLabel),
			[]string{daemonConfirmReconfigureNowLabel, cancelLabel},
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenDaemonActiveConfirm:
		modeLabel := "selected mode"
		switch m.pendingStartMode {
		case config.ModeClient:
			modeLabel = "client"
		case config.ModeServer:
			modeLabel = "server"
		}
		notice := fmt.Sprintf("tungo.service is active. Stop it before starting %s in TUI mode.", modeLabel)
		if strings.TrimSpace(m.notice) != "" {
			notice = m.notice + "\n" + notice
		}
		return m.renderSelectionScreen(
			"Active daemon detected",
			notice,
			[]string{stopDaemonContinueLabel, cancelLabel},
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		)
	case configuratorScreenDaemonCheckErrorConfirm:
		subtitle := "Failed to check daemon status."
		if strings.TrimSpace(m.notice) != "" {
			subtitle = m.notice
		}
		return m.renderSelectionScreen(
			"Cannot verify daemon status",
			subtitle,
			[]string{retryDaemonCheckLabel, startAnywayUnsafeLabel, cancelLabel},
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
		)
	default:
		return ""
	}
}

func (m Configurator) renderSelectionScreen(
	screenTitle string,
	notice string,
	options []string,
	cursor int,
	hint string,
) string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	rows := renderSelectableRows(options, cursor, contentWidth, styles)
	body := make([]string, 0, len(rows)+2)
	if strings.TrimSpace(notice) != "" {
		body = append(body, notice, "")
	}
	body = append(body, rows...)
	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		screenTitle,
		body,
		hint,
		m.preferences,
		styles,
	)
}

func (m Configurator) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return 40 + resolveUIStyles(m.preferences).inputFrame.GetHorizontalFrameSize()
}

func (m Configurator) settingsTabView() string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body := renderSelectableRows(m.settingsRows(), m.settingsCursor, contentWidth, styles)
	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		"up/k down/j row | left/right/Enter change | Tab switch tabs | Esc back | ctrl+c exit",
		m.preferences,
		styles,
	)
}

func (m Configurator) logsTabView() string {
	styles := resolveUIStyles(m.preferences)
	body := []string{m.logs.view()}
	return renderScreen(
		m.width,
		m.height,
		m.tabsLine(styles),
		"",
		body,
		configuratorLogsHint,
		m.preferences,
		styles,
	)
}

func (m Configurator) tabsLine(styles uiStyles) string {
	contentWidth := contentWidthForTerminal(m.width)
	return renderTabsLine(productLabel(), selectorTabs[:], m.tab, contentWidth, styles)
}
