package bubble_tea

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	appConfiguration "tungo/application/configuration"
	"tungo/application/runtime"
	"tungo/infrastructure/PAL/service_management/linux/systemd"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var ErrConfiguratorSessionUserExit = errors.New("configurator session user exit")

const configuratorLogsHint = "up/down scroll | PgUp/PgDn page | Home/End jump | Space follow | Tab switch tabs | Esc back | ctrl+c exit"

type pasteSettledMsg struct {
	seq uint64
}

const (
	configuratorTabMain = iota
	configuratorTabSettings
	configuratorTabLogs
)

type ConfiguratorSessionOptions struct {
	ClientConfigurationControl appConfiguration.ClientConfigurationControl
	ServerConfigurationControl appConfiguration.ServerConfigurationControl
	Daemon                     systemd.Control
}

type configuratorScreen int

const (
	configuratorScreenMode configuratorScreen = iota
	configuratorScreenClientSelect
	configuratorScreenClientRemove
	configuratorScreenClientAddName
	configuratorScreenClientAddJSON
	configuratorScreenClientInvalid
	configuratorScreenServerSelect
	configuratorScreenServerManage
	configuratorScreenServerDeleteConfirm
	configuratorScreenDaemonManage
	configuratorScreenDaemonReconfigureConfirm
	configuratorScreenDaemonActiveConfirm
	configuratorScreenDaemonCheckErrorConfirm
)

const (
	sessionModeClient = "client"
	sessionModeServer = "server"
	sessionModeDaemon = "daemon"

	sessionClientAdd    = "add configuration"
	sessionClientRemove = "remove configuration"

	sessionInvalidDelete = "Delete invalid configuration"
	sessionInvalidOK     = "OK"

	sessionServerStart  = "start server"
	sessionServerAdd    = "add client"
	sessionServerManage = "manage clients"

	sessionDaemonSetupClient           = "setup client daemon"
	sessionDaemonSetupServer           = "setup server daemon"
	sessionDaemonReconfClient          = "reconfigure as client daemon"
	sessionDaemonReconfServer          = "reconfigure as server daemon"
	sessionDaemonStart                 = "start daemon"
	sessionDaemonStop                  = "stop daemon"
	sessionDaemonEnable                = "enable on boot"
	sessionDaemonDisable               = "disable on boot"
	sessionDaemonDelete                = "delete daemon"
	sessionDaemonConfirmReconfigureNow = "stop and restart with new setup"

	sessionServerDeleteConfirm = "Delete client"
	sessionCancel              = "Cancel"
	sessionStopDaemonContinue  = "stop daemon and continue"
	sessionRetryDaemonCheck    = "Retry check"
	sessionStartAnywayUnsafe   = "Start anyway (unsafe)"
)

type clientConfigScreens struct {
	configs            []string
	menuOptions        []string
	removePaths        []string
	addNameInput       textinput.Model
	addJSONInput       textarea.Model
	addName            string
	lastInputAt        time.Time
	pasteSeq           uint64
	invalidErr         error
	invalidConfig      string
	invalidAllowDelete bool
}

type serverConfigScreens struct {
	menuOptions  []string
	managePeers  []appConfiguration.ServerPeer
	manageLabels []string
	deletePeer   appConfiguration.ServerPeer
	deleteCursor int
}

type daemonConfigScreens struct {
	status      systemd.UnitStatus
	statusErr   error
	menuOptions []string
	updatedAt   time.Time
}

type configuratorSessionModel struct {
	settings        *uiPreferencesProvider
	options         ConfiguratorSessionOptions
	serverSupported bool

	width  int
	height int

	screen configuratorScreen
	cursor int

	modeOptions []string
	client      clientConfigScreens
	server      serverConfigScreens
	daemon      daemonConfigScreens

	notice string

	tab            int
	settingsCursor int
	preferences    UIPreferences

	logs logViewport

	pendingStartMode    runtime.Mode
	pendingStartScreen  configuratorScreen
	pendingClientConfig string
	pendingDaemonMode   runtime.Mode

	resultMode runtime.Mode
	resultErr  error
	done       bool
}

func newConfiguratorSessionModel(options ConfiguratorSessionOptions, settings *uiPreferencesProvider) (configuratorSessionModel, error) {
	serverSupported := options.ServerConfigurationControl != nil
	modeOptions := []string{sessionModeClient}
	if serverSupported {
		modeOptions = append(modeOptions, sessionModeServer)
	}
	if options.Daemon != nil {
		modeOptions = append(modeOptions, sessionModeDaemon)
	}

	// If server is not supported but the saved preference is server, reset to client.
	if !serverSupported {
		p := settings.Preferences()
		if p.AutoSelectMode == ModePreferenceServer {
			p.AutoSelectMode = ModePreferenceClient
			settings.update(p)
			_ = savePreferencesToDisk(p)
		}
	}

	model := configuratorSessionModel{
		settings:        settings,
		options:         options,
		serverSupported: serverSupported,
		screen:          configuratorScreenMode,
		cursor:          0,
		modeOptions:     modeOptions,
		server: serverConfigScreens{
			menuOptions: []string{
				sessionServerStart,
				sessionServerAdd,
				sessionServerManage,
			},
		},
		preferences: settings.Preferences(),
		logs:        newLogViewport(),
	}

	if options.ClientConfigurationControl == nil {
		return configuratorSessionModel{}, errors.New("configurator session dependencies are not initialized")
	}
	model.initNameInput()
	model.initJSONInput()
	if options.Daemon != nil {
		model.refreshDaemonStatus()
	}
	modeAutoselectNotice := ""
	switch settings.Preferences().AutoSelectMode {
	case ModePreferenceClient:
		modeAutoselectNotice = "Auto-selected mode: client."
	case ModePreferenceServer:
		modeAutoselectNotice = "Auto-selected mode: server."
	}

	// Skip mode screen only when client is the only available option,
	// or when client is explicitly preferred.
	if len(modeOptions) == 1 || settings.Preferences().AutoSelectMode == ModePreferenceClient {
		if err := model.reloadClientConfigs(); err != nil {
			return configuratorSessionModel{}, err
		}
		model.screen = configuratorScreenClientSelect
		model.notice = appendNotice(model.notice, modeAutoselectNotice)
		if settings.Preferences().AutoConnect {
			if autoConfig := settings.Preferences().AutoSelectClientConfig; autoConfig != "" {
				if slices.Contains(model.client.configs, autoConfig) {
					if err := model.options.ClientConfigurationControl.Select(autoConfig); err == nil {
						model.notice = appendNotice(model.notice, fmt.Sprintf("Auto-selected config: %s.", autoConfig))
						cfgErr := model.options.ClientConfigurationControl.ValidateActive()
						if isInvalidClientConfigurationError(cfgErr) {
							model.client.invalidErr = cfgErr
							model.client.invalidConfig = autoConfig
							model.client.invalidAllowDelete = true
							model.cursor = 0
							model.screen = configuratorScreenClientInvalid
						} else if cfgErr != nil {
							model.notice = fmt.Sprintf("Auto-select failed for %q: %v", autoConfig, cfgErr)
						} else {
							model = model.startModeWithDaemonGuard(runtime.ModeClient, configuratorScreenClientSelect, true)
							if !model.done && isDaemonStartConfirmationScreen(model.screen) {
								model.pendingClientConfig = autoConfig
							}
						}
					} else {
						model.notice = fmt.Sprintf("Auto-select failed for %q: %v", autoConfig, err)
					}
				} else {
					p := settings.Preferences()
					p.AutoSelectClientConfig = ""
					settings.update(p)
					_ = savePreferencesToDisk(p)
				}
			}
		}
	} else if settings.Preferences().AutoSelectMode == ModePreferenceServer {
		model.screen = configuratorScreenServerSelect
		model.notice = appendNotice(model.notice, modeAutoselectNotice)
	}

	return model, nil
}

func (m configuratorSessionModel) Init() tea.Cmd {
	return nil
}

func (m configuratorSessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		m.logs.stopWait()
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustInputsToViewport()
		if m.tab == configuratorTabLogs {
			m.logs.ensure(m.width, m.height, m.preferences, "", configuratorLogsHint)
			m.logs.refresh(globalRuntimeLogFeed(), m.preferences)
		}
		return m, nil
	case logViewportTickMsg:
		if msg.seq != m.logs.tickSeq || m.tab != configuratorTabLogs {
			return m, nil
		}
		feed := globalRuntimeLogFeed()
		m.logs.refresh(feed, m.preferences)
		return m, logViewportUpdateCmd(feed, m.logs.waitStop, m.logs.tickSeq)
	case pasteSettledMsg:
		if m.screen == configuratorScreenClientAddJSON && msg.seq == m.client.pasteSeq {
			m.tryFormatJSON()
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.logs.stopWait()
			m.resultErr = ErrConfiguratorSessionUserExit
			m.done = true
			return m, tea.Quit
		case "tab":
			if m.screen != configuratorScreenClientAddName && m.screen != configuratorScreenClientAddJSON {
				return m.cycleTab()
			}
		}

		switch m.tab {
		case configuratorTabSettings:
			return m.updateSettingsTab(msg)
		case configuratorTabLogs:
			return m.updateLogsTab(msg)
		}

		switch m.screen {
		case configuratorScreenMode:
			return m.updateModeScreen(msg)
		case configuratorScreenClientSelect:
			return m.updateClientSelectScreen(msg)
		case configuratorScreenClientRemove:
			return m.updateClientRemoveScreen(msg)
		case configuratorScreenClientAddName:
			return m.updateClientAddNameScreen(msg)
		case configuratorScreenClientAddJSON:
			return m.updateClientAddJSONScreen(msg)
		case configuratorScreenClientInvalid:
			return m.updateClientInvalidScreen(msg)
		case configuratorScreenServerSelect:
			return m.updateServerSelectScreen(msg)
		case configuratorScreenServerManage:
			return m.updateServerManageScreen(msg)
		case configuratorScreenServerDeleteConfirm:
			return m.updateServerDeleteConfirmScreen(msg)
		case configuratorScreenDaemonManage:
			return m.updateDaemonManageScreen(msg)
		case configuratorScreenDaemonReconfigureConfirm:
			return m.updateDaemonReconfigureConfirmScreen(msg)
		case configuratorScreenDaemonActiveConfirm:
			return m.updateDaemonActiveConfirmScreen(msg)
		case configuratorScreenDaemonCheckErrorConfirm:
			return m.updateDaemonCheckErrorConfirmScreen(msg)
		}
	}

	// Forward non-key messages (e.g. clipboard paste results, cursor blink ticks)
	// to the active input component so they are not silently dropped.
	switch m.screen {
	case configuratorScreenClientAddName:
		var cmd tea.Cmd
		m.client.addNameInput, cmd = m.client.addNameInput.Update(msg)
		return m, cmd
	case configuratorScreenClientAddJSON:
		var cmd tea.Cmd
		m.client.addJSONInput, cmd = m.client.addJSONInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m configuratorSessionModel) View() tea.View {
	var content string
	switch m.tab {
	case configuratorTabSettings:
		content = m.settingsTabView()
	case configuratorTabLogs:
		content = m.logsTabView()
	default:
		content = m.mainTabView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m configuratorSessionModel) updateModeScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.resultErr = ErrConfiguratorSessionUserExit
		m.done = true
		return m, tea.Quit
	}

	m.updateCursor(msg, len(m.modeOptions))
	if msg.String() != "enter" {
		return m, nil
	}

	switch m.modeOptions[m.cursor] {
	case sessionModeClient:
		if err := m.reloadClientConfigs(); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
	case sessionModeServer:
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenServerSelect
	case sessionModeDaemon:
		m.notice = ""
		m.cursor = 0
		m.refreshDaemonStatus()
		m.screen = configuratorScreenDaemonManage
	}
	return m, nil
}

func appendNotice(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return next
	}
	return existing + "\n" + next
}

func (m configuratorSessionModel) cycleTab() (tea.Model, tea.Cmd) {
	previous := m.tab
	switch m.tab {
	case configuratorTabMain:
		m.tab = configuratorTabSettings
	case configuratorTabSettings:
		m.tab = configuratorTabLogs
	default:
		m.tab = configuratorTabMain
	}
	m.preferences = m.settings.Preferences()
	if m.tab == configuratorTabLogs {
		m.logs.restartWait()
		m.logs.tickSeq++
		m.logs.ensure(m.width, m.height, m.preferences, "", configuratorLogsHint)
		feed := globalRuntimeLogFeed()
		m.logs.refresh(feed, m.preferences)
		return m, logViewportUpdateCmd(feed, m.logs.waitStop, m.logs.tickSeq)
	}
	if previous == configuratorTabLogs {
		m.logs.stopWait()
	}
	return m, nil
}

func (m configuratorSessionModel) updateSettingsTab(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tab = configuratorTabMain
		return m, nil
	}
	rows := m.settingsRows()
	if len(rows) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	switch msg.String() {
	case "up", "k":
		m.settingsCursor = settingsCursorUp(m.settingsCursor)
	case "down", "j":
		m.settingsCursor = settingsCursorDown(m.settingsCursor, len(rows))
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
	if m.settingsCursor >= len(m.settingsRows()) {
		m.settingsCursor = maxInt(0, len(m.settingsRows())-1)
	}
	return m, cmd
}

func (m configuratorSessionModel) updateLogsTab(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.logs.stopWait()
		m.tab = configuratorTabMain
		return m, nil
	}
	return m, m.logs.updateKeys(msg, defaultSelectorKeyMap())
}

func (m configuratorSessionModel) settingsRows() []string {
	return uiSettingsRows(m.preferences, m.serverSupported)
}

func (m *configuratorSessionModel) updateCursor(keyMsg tea.KeyMsg, listSize int) {
	if listSize <= 0 {
		m.cursor = 0
		return
	}

	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < listSize-1 {
			m.cursor++
		}
	}
}
