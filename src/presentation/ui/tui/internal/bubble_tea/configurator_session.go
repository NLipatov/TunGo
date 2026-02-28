package bubble_tea

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tungo/application/confgen"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"

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

type configuratorSessionProgram interface {
	Run() (tea.Model, error)
}

var newConfiguratorSessionProgram = func(model tea.Model) configuratorSessionProgram {
	return tea.NewProgram(model)
}

var resolveServerConfigDir = func() (string, error) {
	configPath, err := serverConfiguration.NewServerResolver().Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Dir(configPath), nil
}

var writeServerClientConfigFile = func(clientID int, data []byte) (string, error) {
	dir, err := resolveServerConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve server config directory: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("client_configuration.json.%d", clientID))
	return path, os.WriteFile(path, data, 0600)
}

type ConfiguratorSessionOptions struct {
	Observer            clientConfiguration.Observer
	Selector            clientConfiguration.Selector
	Creator             clientConfiguration.Creator
	Deleter             clientConfiguration.Deleter
	ClientConfigManager clientConfiguration.ConfigurationManager
	ServerConfigManager serverConfiguration.ConfigurationManager
	ServerSupported     bool
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
)

const (
	sessionModeClient = "client"
	sessionModeServer = "server"

	sessionClientAdd    = "add configuration"
	sessionClientRemove = "remove configuration"

	sessionInvalidDelete = "Delete invalid configuration"
	sessionInvalidOK     = "OK"

	sessionServerStart  = "start server"
	sessionServerAdd    = "add client"
	sessionServerManage = "manage clients"

	sessionServerDeleteConfirm = "Delete client"
	sessionCancel              = "Cancel"
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
	managePeers  []serverConfiguration.AllowedPeer
	manageLabels []string
	deletePeer   serverConfiguration.AllowedPeer
	deleteCursor int
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

	notice string

	tab            int
	settingsCursor int
	preferences    UIPreferences

	logs logViewport

	resultMode mode.Mode
	resultErr  error
	done       bool
}

func RunConfiguratorSession(options ConfiguratorSessionOptions) (selectedMode mode.Mode, err error) {
	defer clearTerminalAfterTUI()

	settings := loadUISettingsFromDisk()
	model, err := newConfiguratorSessionModel(options, settings)
	if err != nil {
		return mode.Unknown, err
	}

	program := newConfiguratorSessionProgram(model)
	finalModel, runErr := program.Run()
	if runErr != nil {
		return mode.Unknown, runErr
	}

	result, ok := finalModel.(configuratorSessionModel)
	if !ok {
		return mode.Unknown, errors.New("invalid configurator session model")
	}
	if result.resultErr != nil {
		return mode.Unknown, result.resultErr
	}
	return result.resultMode, nil
}

func newConfiguratorSessionModel(options ConfiguratorSessionOptions, settings *uiPreferencesProvider) (configuratorSessionModel, error) {
	modeOptions := []string{sessionModeClient}
	if options.ServerSupported {
		modeOptions = append(modeOptions, sessionModeServer)
	}

	// If server is not supported but the saved preference is server, reset to client.
	if !options.ServerSupported {
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
		serverSupported: options.ServerSupported,
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

	if options.Observer == nil ||
		options.Selector == nil ||
		options.Creator == nil ||
		options.Deleter == nil ||
		options.ServerConfigManager == nil {
		return configuratorSessionModel{}, errors.New("configurator session dependencies are not initialized")
	}

	model.initNameInput()
	model.initJSONInput()

	switch settings.Preferences().AutoSelectMode {
	case ModePreferenceClient:
		if err := model.reloadClientConfigs(); err != nil {
			return configuratorSessionModel{}, err
		}
		model.screen = configuratorScreenClientSelect
		if autoConfig := settings.Preferences().AutoSelectClientConfig; autoConfig != "" {
			if slices.Contains(model.client.configs, autoConfig) {
				if err := model.options.Selector.Select(autoConfig); err == nil {
					model.resultMode = mode.Client
					model.done = true
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
	case ModePreferenceServer:
		model.screen = configuratorScreenServerSelect
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
			m.logs.refresh(m.logsFeed(), m.preferences)
		}
		return m, nil
	case logViewportTickMsg:
		if msg.seq != m.logs.tickSeq || m.tab != configuratorTabLogs {
			return m, nil
		}
		m.logs.refresh(m.logsFeed(), m.preferences)
		return m, configuratorLogUpdateCmd(m.logsFeed(), m.logs.waitStop, m.logs.tickSeq)
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

func (m configuratorSessionModel) mainTabView() string {
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
		return m.renderSelectionScreen(
			"Select configuration - or add/remove one:",
			m.notice,
			m.client.menuOptions,
			m.cursor,
			"up/k down/j move | Enter select | Tab switch tabs | Esc back | ctrl+c exit",
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
		options := []string{sessionInvalidOK}
		if m.client.invalidAllowDelete {
			options = []string{sessionInvalidDelete, sessionInvalidOK}
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
			[]string{sessionServerDeleteConfirm, sessionCancel},
			m.cursor,
			"up/k down/j move | Enter confirm | Tab switch tabs | Esc back | ctrl+c exit",
		)
	default:
		return ""
	}
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
	}
	return m, nil
}

func (m configuratorSessionModel) updateClientSelectScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenMode
		return m, nil
	}

	m.updateCursor(msg, len(m.client.menuOptions))
	if msg.String() != "enter" || len(m.client.menuOptions) == 0 {
		return m, nil
	}

	selected := m.client.menuOptions[m.cursor]
	switch selected {
	case sessionClientAdd:
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientAddName
		m.initNameInput()
		m.adjustInputsToViewport()
		return m, textinput.Blink
	case sessionClientRemove:
		if len(m.client.configs) == 0 {
			m.notice = "No configurations available for removal."
			return m, nil
		}
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientRemove
		m.client.removePaths = append([]string(nil), m.client.configs...)
		return m, nil
	default:
		if err := m.options.Selector.Select(selected); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}

		if m.options.ClientConfigManager != nil {
			_, cfgErr := m.options.ClientConfigManager.Configuration()
			if isInvalidClientConfigurationError(cfgErr) {
				m.client.invalidErr = cfgErr
				m.client.invalidConfig = selected
				m.client.invalidAllowDelete = true
				m.cursor = 0
				m.screen = configuratorScreenClientInvalid
				return m, nil
			}
			if cfgErr != nil {
				m.resultErr = cfgErr
				m.done = true
				return m, tea.Quit
			}
		}

		p := m.settings.Preferences()
		p.AutoSelectClientConfig = selected
		m.settings.update(p)
		_ = savePreferencesToDisk(p)
		m.resultMode = mode.Client
		m.done = true
		return m, tea.Quit
	}
}

func (m configuratorSessionModel) updateClientRemoveScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	m.updateCursor(msg, len(m.client.removePaths))
	if msg.String() != "enter" || len(m.client.removePaths) == 0 {
		return m, nil
	}

	toDelete := m.client.removePaths[m.cursor]
	if err := m.options.Deleter.Delete(toDelete); err != nil {
		m.resultErr = err
		m.done = true
		return m, tea.Quit
	}
	if err := m.reloadClientConfigs(); err != nil {
		m.resultErr = err
		m.done = true
		return m, tea.Quit
	}
	m.notice = "Configuration removed."
	m.cursor = 0
	m.screen = configuratorScreenClientSelect
	return m, nil
}

func (m configuratorSessionModel) updateClientAddNameScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.client.addNameInput.Value())
		if name == "" {
			m.notice = "Configuration name cannot be empty."
			return m, nil
		}
		m.client.addName = name
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientAddJSON
		m.client.lastInputAt = time.Time{}
		m.initJSONInput()
		m.adjustInputsToViewport()
		return m, textarea.Blink
	}

	var cmd tea.Cmd
	m.client.addNameInput, cmd = m.client.addNameInput.Update(msg)
	return m, cmd
}

const pasteDebounce = 300 * time.Millisecond

func (m configuratorSessionModel) updateClientAddJSONScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.notice = ""
		m.screen = configuratorScreenClientAddName
		m.adjustInputsToViewport()
		return m, nil
	}

	if msg.String() == "enter" {
		// Debounce: if Enter arrives within pasteDebounce of the last
		// non-Enter keystroke, it is almost certainly a newline from a
		// character-by-character terminal paste — insert it as a newline
		// instead of submitting.
		if !m.client.lastInputAt.IsZero() && time.Since(m.client.lastInputAt) < pasteDebounce {
			m.client.lastInputAt = time.Now()
			var cmd tea.Cmd
			m.client.addJSONInput, cmd = m.client.addJSONInput.Update(msg)
			return m, cmd
		}

		configuration, parseErr := parseClientConfigurationJSON(m.client.addJSONInput.Value())
		if parseErr != nil {
			m.client.invalidErr = parseErr
			m.client.invalidConfig = ""
			m.client.invalidAllowDelete = false
			m.cursor = 0
			m.screen = configuratorScreenClientInvalid
			return m, nil
		}

		if err := m.options.Creator.Create(configuration, m.client.addName); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		if err := m.reloadClientConfigs(); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}

		m.notice = "Configuration added."
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	// Track non-Enter input timing for debounce.
	m.client.lastInputAt = time.Now()
	m.client.pasteSeq++
	seq := m.client.pasteSeq

	// Forward to textarea (paste characters, cursor movement, etc.)
	var cmd tea.Cmd
	m.client.addJSONInput, cmd = m.client.addJSONInput.Update(msg)
	return m, tea.Batch(cmd, tea.Tick(pasteDebounce, func(time.Time) tea.Msg {
		return pasteSettledMsg{seq: seq}
	}))
}

func (m configuratorSessionModel) updateClientInvalidScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	options := []string{sessionInvalidOK}
	if m.client.invalidAllowDelete {
		options = []string{sessionInvalidDelete, sessionInvalidOK}
	}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" || len(options) == 0 {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == sessionInvalidDelete && m.client.invalidAllowDelete {
		if strings.TrimSpace(m.client.invalidConfig) == "" {
			m.resultErr = errors.New("invalid configuration cannot be deleted")
			m.done = true
			return m, tea.Quit
		}
		if err := m.options.Deleter.Delete(m.client.invalidConfig); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		if err := m.reloadClientConfigs(); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		m.notice = "Invalid configuration deleted."
	}
	m.cursor = 0
	m.screen = configuratorScreenClientSelect
	return m, nil
}

func (m configuratorSessionModel) updateServerSelectScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenMode
		return m, nil
	}

	m.updateCursor(msg, len(m.server.menuOptions))
	if msg.String() != "enter" || len(m.server.menuOptions) == 0 {
		return m, nil
	}

	switch m.server.menuOptions[m.cursor] {
	case sessionServerStart:
		m.resultMode = mode.Server
		m.done = true
		return m, tea.Quit
	case sessionServerAdd:
		gen := confgen.NewGenerator(m.options.ServerConfigManager, &primitives.DefaultKeyDeriver{})
		conf, err := gen.Generate()
		if err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		data, err := json.MarshalIndent(conf, "", "  ")
		if err != nil {
			m.resultErr = fmt.Errorf("failed to marshal client configuration: %w", err)
			m.done = true
			return m, tea.Quit
		}
		path, fileErr := writeServerClientConfigFile(conf.ClientID, data)
		if fileErr != nil {
			m.resultErr = fmt.Errorf("failed to save client configuration: %w", fileErr)
			m.done = true
			return m, tea.Quit
		}
		m.notice = fmt.Sprintf("Client configuration saved to %s", path)
		return m, nil
	case sessionServerManage:
		peers, err := m.options.ServerConfigManager.ListAllowedPeers()
		if err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		if len(peers) == 0 {
			m.notice = "No clients configured yet."
			return m, nil
		}
		m.server.managePeers = peers
		m.server.manageLabels = buildServerManageLabels(peers)
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenServerManage
		return m, nil
	}
	return m, nil
}

func (m configuratorSessionModel) updateServerManageScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenServerSelect
		return m, nil
	case "d", "D":
		if len(m.server.managePeers) == 0 {
			return m, nil
		}
		m.server.deletePeer = m.server.managePeers[m.cursor]
		m.server.deleteCursor = m.cursor
		m.cursor = 0
		m.screen = configuratorScreenServerDeleteConfirm
		return m, nil
	}

	m.updateCursor(msg, len(m.server.managePeers))
	if msg.String() != "enter" || len(m.server.managePeers) == 0 {
		return m, nil
	}

	peer := m.server.managePeers[m.cursor]
	nextEnabled := !peer.Enabled
	if err := m.options.ServerConfigManager.SetAllowedPeerEnabled(peer.ClientID, nextEnabled); err != nil {
		m.notice = fmt.Sprintf("Failed to update client #%d: %v", peer.ClientID, err)
		m.screen = configuratorScreenServerSelect
		m.cursor = 0
		return m, nil
	}

	peers, err := m.options.ServerConfigManager.ListAllowedPeers()
	if err != nil {
		m.resultErr = err
		m.done = true
		return m, tea.Quit
	}
	if len(peers) == 0 {
		m.notice = "No clients configured yet."
		m.screen = configuratorScreenServerSelect
		m.cursor = 0
		return m, nil
	}

	m.server.managePeers = peers
	m.server.manageLabels = buildServerManageLabels(peers)
	if m.cursor >= len(m.server.managePeers) {
		m.cursor = len(m.server.managePeers) - 1
	}
	return m, nil
}

func (m configuratorSessionModel) updateServerDeleteConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(m.server.managePeers) > 0 {
			m.cursor = minInt(m.server.deleteCursor, len(m.server.managePeers)-1)
		} else {
			m.cursor = 0
		}
		m.screen = configuratorScreenServerManage
		return m, nil
	}

	options := []string{sessionServerDeleteConfirm, sessionCancel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == sessionCancel {
		if len(m.server.managePeers) > 0 {
			m.cursor = minInt(m.server.deleteCursor, len(m.server.managePeers)-1)
		} else {
			m.cursor = 0
		}
		m.screen = configuratorScreenServerManage
		return m, nil
	}

	if err := m.options.ServerConfigManager.RemoveAllowedPeer(m.server.deletePeer.ClientID); err != nil {
		m.notice = fmt.Sprintf("Failed to remove client #%d: %v", m.server.deletePeer.ClientID, err)
		m.screen = configuratorScreenServerManage
		m.cursor = 0
		return m, nil
	}

	peers, err := m.options.ServerConfigManager.ListAllowedPeers()
	if err != nil {
		m.resultErr = err
		m.done = true
		return m, tea.Quit
	}
	if len(peers) == 0 {
		m.notice = "No clients configured yet."
		m.screen = configuratorScreenServerSelect
		m.cursor = 0
		return m, nil
	}

	m.notice = fmt.Sprintf(
		"Client #%d %s removed.",
		m.server.deletePeer.ClientID,
		serverPeerDisplayName(m.server.deletePeer),
	)
	m.server.managePeers = peers
	m.server.manageLabels = buildServerManageLabels(peers)
	m.cursor = minInt(m.server.deleteCursor, len(peers)-1)
	m.screen = configuratorScreenServerManage
	return m, nil
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
		m.logs.refresh(m.logsFeed(), m.preferences)
		return m, configuratorLogUpdateCmd(m.logsFeed(), m.logs.waitStop, m.logs.tickSeq)
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
	var cmd tea.Cmd
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

func (m *configuratorSessionModel) reloadClientConfigs() error {
	configs, err := m.options.Observer.Observe()
	if err != nil {
		return err
	}
	m.client.configs = configs
	m.client.menuOptions = make([]string, 0, len(configs)+2)
	m.client.menuOptions = append(m.client.menuOptions, configs...)
	if len(configs) > 0 {
		m.client.menuOptions = append(m.client.menuOptions, sessionClientRemove)
	}
	m.client.menuOptions = append(m.client.menuOptions, sessionClientAdd)
	return nil
}

func (m *configuratorSessionModel) initNameInput() {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Give it a name"
	ti.CharLimit = 256
	ti.SetWidth(40)
	ti.SetValue("")
	ti.Focus()
	m.client.addNameInput = ti
}

func (m *configuratorSessionModel) tryFormatJSON() {
	raw := m.client.addJSONInput.Value()
	if strings.TrimSpace(raw) == "" {
		return
	}
	var obj json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return
	}
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return
	}
	if string(pretty) != raw {
		m.client.addJSONInput.SetValue(string(pretty))
	}
}

func (m *configuratorSessionModel) initJSONInput() {
	ta := textarea.New()
	ta.Prompt = "> "
	ta.Placeholder = "Paste it here"
	ta.SetWidth(80)
	ta.SetHeight(10)
	ta.ShowLineNumbers = true
	styles := ta.Styles()
	styles.Focused.CursorLine = styles.Focused.Text
	ta.SetStyles(styles)
	ta.SetValue("")
	ta.Focus()
	m.client.addJSONInput = ta
}

func (m *configuratorSessionModel) adjustInputsToViewport() {
	if m.width <= 0 {
		return
	}
	contentWidth := contentWidthForTerminal(m.width)
	available := maxInt(1, contentWidth-resolveUIStyles(m.preferences).inputFrame.GetHorizontalFrameSize())
	m.client.addNameInput.SetWidth(minInt(40, available))
	m.client.addJSONInput.SetWidth(minInt(80, available))
	if m.height > 18 {
		m.client.addJSONInput.SetHeight(m.height - 18)
	}
}

func (m configuratorSessionModel) renderSelectionScreen(
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

func (m configuratorSessionModel) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return 40 + resolveUIStyles(m.preferences).inputFrame.GetHorizontalFrameSize()
}

func (m configuratorSessionModel) settingsTabView() string {
	styles := resolveUIStyles(m.preferences)
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}
	body := renderSelectableRows(uiSettingsRows(m.preferences, m.serverSupported), m.settingsCursor, contentWidth, styles)
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

func (m configuratorSessionModel) logsTabView() string {
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

func (m configuratorSessionModel) tabsLine(styles uiStyles) string {
	contentWidth := contentWidthForTerminal(m.width)
	return renderTabsLine(productLabel(), "configurator", selectorTabs[:], m.tab, contentWidth, m.preferences.Theme, styles)
}

func (m configuratorSessionModel) logsFeed() RuntimeLogFeed {
	return GlobalRuntimeLogFeed()
}

func configuratorLogUpdateCmd(feed RuntimeLogFeed, stop <-chan struct{}, seq uint64) tea.Cmd {
	return logViewportUpdateCmd(feed, stop, seq)
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

func buildServerManageLabels(peers []serverConfiguration.AllowedPeer) []string {
	labels := make([]string, 0, len(peers))
	for _, peer := range peers {
		labels = append(labels, serverPeerOptionLabel(peer))
	}
	return labels
}

func serverPeerDisplayName(peer serverConfiguration.AllowedPeer) string {
	name := strings.TrimSpace(peer.Name)
	if name == "" {
		return fmt.Sprintf("client-%d", peer.ClientID)
	}
	return name
}

func serverPeerOptionLabel(peer serverConfiguration.AllowedPeer) string {
	status := "disabled"
	if peer.Enabled {
		status = "enabled"
	}
	name := serverPeerDisplayName(peer)
	return fmt.Sprintf("#%d %s [%s]", peer.ClientID, name, status)
}

func parseClientConfigurationJSON(input string) (clientConfiguration.Configuration, error) {
	sanitized := sanitizeConfigurationJSON(input)
	clean := strings.TrimSpace(sanitized)
	var cfg clientConfiguration.Configuration
	if err := json.Unmarshal([]byte(clean), &cfg); err != nil {
		return clientConfiguration.Configuration{}, err
	}
	if err := cfg.Validate(); err != nil {
		return clientConfiguration.Configuration{}, err
	}
	return cfg, nil
}

func sanitizeConfigurationJSON(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func summarizeInvalidConfigurationError(err error) string {
	if err == nil {
		return ""
	}

	message := strings.TrimSpace(err.Error())
	normalized := strings.ToLower(message)
	if strings.Contains(normalized, "invalid client configuration (") {
		if separatorIdx := strings.Index(message, "): "); separatorIdx >= 0 && separatorIdx+3 <= len(message) {
			message = message[separatorIdx+3:]
		}
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 120 {
		return message[:117] + "..."
	}
	return message
}

func isInvalidClientConfigurationError(err error) bool {
	if err == nil {
		return false
	}

	normalized := strings.ToLower(err.Error())
	invalidMessages := []string{
		"invalid client configuration",
		"invalid character",
		"cannot unmarshal",
		"unexpected eof",
	}
	for _, message := range invalidMessages {
		if strings.Contains(normalized, message) {
			return true
		}
	}
	return false
}
