package bubble_tea

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"tungo/application/confgen"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var ErrConfiguratorSessionUserExit = errors.New("configurator session user exit")

type ConfiguratorSessionOptions struct {
	Observer            clientConfiguration.Observer
	Selector            clientConfiguration.Selector
	Creator             clientConfiguration.Creator
	Deleter             clientConfiguration.Deleter
	ClientConfigManager clientConfiguration.ConfigurationManager
	ServerConfigManager serverConfiguration.ConfigurationManager
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
)

const (
	sessionModeClient = "client"
	sessionModeServer = "server"

	sessionClientAdd    = "+ add configuration"
	sessionClientRemove = "- remove configuration"

	sessionInvalidDelete = "Delete invalid configuration"
	sessionInvalidOK     = "OK"

	sessionServerStart  = "start server"
	sessionServerAdd    = "add client"
	sessionServerManage = "manage clients"
)

type configuratorSessionModel struct {
	options ConfiguratorSessionOptions

	width  int
	height int

	screen configuratorScreen
	cursor int

	modeOptions        []string
	clientConfigs      []string
	clientMenuOptions  []string
	clientRemovePaths  []string
	serverMenuOptions  []string
	serverManagePeers  []serverConfiguration.AllowedPeer
	serverManageLabels []string

	addNameInput textinput.Model
	addJSONInput textarea.Model
	addName      string

	invalidErr         error
	invalidConfig      string
	invalidAllowDelete bool

	notice string

	resultMode mode.Mode
	resultErr  error
	done       bool
}

func RunConfiguratorSession(options ConfiguratorSessionOptions) (mode.Mode, error) {
	defer clearTerminalAfterTUI()

	model, err := newConfiguratorSessionModel(options)
	if err != nil {
		return mode.Unknown, err
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
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

func newConfiguratorSessionModel(options ConfiguratorSessionOptions) (configuratorSessionModel, error) {
	model := configuratorSessionModel{
		options: options,
		screen:  configuratorScreenMode,
		cursor:  0,
		modeOptions: []string{
			sessionModeClient,
			sessionModeServer,
		},
		serverMenuOptions: []string{
			sessionServerStart,
			sessionServerAdd,
			sessionServerManage,
		},
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
	return model, nil
}

func (m configuratorSessionModel) Init() tea.Cmd {
	return nil
}

func (m configuratorSessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustInputsToViewport()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.resultErr = ErrConfiguratorSessionUserExit
			m.done = true
			return m, tea.Quit
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
		}
	}

	return m, nil
}

func (m configuratorSessionModel) View() string {
	switch m.screen {
	case configuratorScreenMode:
		return m.renderSelectionScreen(
			"Select mode",
			m.notice,
			m.modeOptions,
			m.cursor,
			"up/k move | down/j move | Enter select | Esc exit | q exit",
		)
	case configuratorScreenClientSelect:
		return m.renderSelectionScreen(
			"Select configuration - or add/remove one:",
			m.notice,
			m.clientMenuOptions,
			m.cursor,
			"up/k move | down/j move | Enter select | Esc back | q exit",
		)
	case configuratorScreenClientRemove:
		return m.renderSelectionScreen(
			"Choose a configuration to remove:",
			"",
			m.clientRemovePaths,
			m.cursor,
			"up/k move | down/j move | Enter remove | Esc back | q exit",
		)
	case configuratorScreenClientAddName:
		container := inputContainerStyle().Width(m.inputContainerWidth())
		stats := metaTextStyle().Render("Characters: " + formatCount(utf8.RuneCountInString(m.addNameInput.Value()), m.addNameInput.CharLimit))
		return renderScreen(
			m.width,
			m.height,
			"Name configuration",
			m.notice,
			[]string{
				container.Render(m.addNameInput.View()),
				stats,
			},
			"Enter confirm | Esc back | q exit",
		)
	case configuratorScreenClientAddJSON:
		container := inputContainerStyle().Width(m.inputContainerWidth())
		lines := 1
		if value := m.addJSONInput.Value(); value != "" {
			lines = len(strings.Split(value, "\n"))
		}
		stats := metaTextStyle().Render(fmt.Sprintf("Lines: %d", lines))
		return renderScreen(
			m.width,
			m.height,
			"Paste configuration",
			m.notice,
			[]string{
				container.Render(m.addJSONInput.View()),
				stats,
			},
			"Enter confirm | Esc back | q exit",
		)
	case configuratorScreenClientInvalid:
		options := []string{sessionInvalidOK}
		if m.invalidAllowDelete {
			options = []string{sessionInvalidDelete, sessionInvalidOK}
		}
		subtitle := "Configuration is invalid: " + summarizeInvalidConfigurationError(m.invalidErr)
		return m.renderSelectionScreen(
			"Configuration error",
			subtitle,
			options,
			m.cursor,
			"up/k move | down/j move | Enter select | Esc back | q exit",
		)
	case configuratorScreenServerSelect:
		return m.renderSelectionScreen(
			"Choose an option",
			m.notice,
			m.serverMenuOptions,
			m.cursor,
			"up/k move | down/j move | Enter select | Esc back | q exit",
		)
	case configuratorScreenServerManage:
		return m.renderSelectionScreen(
			"Select client to enable/disable",
			"",
			m.serverManageLabels,
			m.cursor,
			"up/k move | down/j move | Enter toggle | Esc back | q exit",
		)
	default:
		return ""
	}
}

func (m configuratorSessionModel) updateModeScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m configuratorSessionModel) updateClientSelectScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenMode
		return m, nil
	}

	m.updateCursor(msg, len(m.clientMenuOptions))
	if msg.String() != "enter" || len(m.clientMenuOptions) == 0 {
		return m, nil
	}

	selected := m.clientMenuOptions[m.cursor]
	switch selected {
	case sessionClientAdd:
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientAddName
		m.initNameInput()
		m.adjustInputsToViewport()
		return m, textinput.Blink
	case sessionClientRemove:
		if len(m.clientConfigs) == 0 {
			m.notice = "No configurations available for removal."
			return m, nil
		}
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientRemove
		m.clientRemovePaths = append([]string(nil), m.clientConfigs...)
		return m, nil
	default:
		if err := m.options.Selector.Select(selected); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}

		if m.options.ClientConfigManager == nil {
			m.resultMode = mode.Client
			m.done = true
			return m, tea.Quit
		}

		_, cfgErr := m.options.ClientConfigManager.Configuration()
		if cfgErr == nil {
			m.resultMode = mode.Client
			m.done = true
			return m, tea.Quit
		}
		if !isInvalidClientConfigurationError(cfgErr) {
			m.resultErr = cfgErr
			m.done = true
			return m, tea.Quit
		}

		m.invalidErr = cfgErr
		m.invalidConfig = selected
		m.invalidAllowDelete = true
		m.cursor = 0
		m.screen = configuratorScreenClientInvalid
		return m, nil
	}
}

func (m configuratorSessionModel) updateClientRemoveScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	m.updateCursor(msg, len(m.clientRemovePaths))
	if msg.String() != "enter" || len(m.clientRemovePaths) == 0 {
		return m, nil
	}

	toDelete := m.clientRemovePaths[m.cursor]
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

func (m configuratorSessionModel) updateClientAddNameScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.addNameInput.Value())
		if name == "" {
			m.notice = "Configuration name cannot be empty."
			return m, nil
		}
		m.addName = name
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientAddJSON
		m.initJSONInput()
		m.adjustInputsToViewport()
		return m, textarea.Blink
	}

	var cmd tea.Cmd
	m.addNameInput, cmd = m.addNameInput.Update(msg)
	return m, cmd
}

func (m configuratorSessionModel) updateClientAddJSONScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.screen = configuratorScreenClientAddName
		m.adjustInputsToViewport()
		return m, nil
	case "enter":
		configuration, parseErr := parseClientConfigurationJSON(m.addJSONInput.Value())
		if parseErr != nil {
			m.invalidErr = parseErr
			m.invalidConfig = ""
			m.invalidAllowDelete = false
			m.cursor = 0
			m.screen = configuratorScreenClientInvalid
			return m, nil
		}

		if err := m.options.Creator.Create(configuration, m.addName); err != nil {
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

	var cmd tea.Cmd
	m.addJSONInput, cmd = m.addJSONInput.Update(msg)
	return m, cmd
}

func (m configuratorSessionModel) updateClientInvalidScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	options := []string{sessionInvalidOK}
	if m.invalidAllowDelete {
		options = []string{sessionInvalidDelete, sessionInvalidOK}
	}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" || len(options) == 0 {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == sessionInvalidDelete && m.invalidAllowDelete {
		if strings.TrimSpace(m.invalidConfig) == "" {
			m.resultErr = errors.New("invalid configuration cannot be deleted")
			m.done = true
			return m, tea.Quit
		}
		if err := m.options.Deleter.Delete(m.invalidConfig); err != nil {
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

func (m configuratorSessionModel) updateServerSelectScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenMode
		return m, nil
	}

	m.updateCursor(msg, len(m.serverMenuOptions))
	if msg.String() != "enter" || len(m.serverMenuOptions) == 0 {
		return m, nil
	}

	switch m.serverMenuOptions[m.cursor] {
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
		if err := clipboard.WriteAll(string(data)); err != nil {
			m.resultErr = fmt.Errorf("failed to copy client configuration to clipboard: %w", err)
			m.done = true
			return m, tea.Quit
		}
		m.notice = "Client configuration copied to clipboard."
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
		m.serverManagePeers = peers
		m.serverManageLabels = buildServerManageLabels(peers)
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenServerManage
		return m, nil
	}
	return m, nil
}

func (m configuratorSessionModel) updateServerManageScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenServerSelect
		return m, nil
	}

	m.updateCursor(msg, len(m.serverManagePeers))
	if msg.String() != "enter" || len(m.serverManagePeers) == 0 {
		return m, nil
	}

	peer := m.serverManagePeers[m.cursor]
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

	m.serverManagePeers = peers
	m.serverManageLabels = buildServerManageLabels(peers)
	if m.cursor >= len(m.serverManagePeers) {
		m.cursor = len(m.serverManagePeers) - 1
	}
	return m, nil
}

func (m *configuratorSessionModel) reloadClientConfigs() error {
	configs, err := m.options.Observer.Observe()
	if err != nil {
		return err
	}
	m.clientConfigs = configs
	m.clientMenuOptions = make([]string, 0, len(configs)+2)
	m.clientMenuOptions = append(m.clientMenuOptions, configs...)
	if len(configs) > 0 {
		m.clientMenuOptions = append(m.clientMenuOptions, sessionClientRemove)
	}
	m.clientMenuOptions = append(m.clientMenuOptions, sessionClientAdd)
	return nil
}

func (m *configuratorSessionModel) initNameInput() {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Give it a name"
	ti.CharLimit = 256
	ti.Width = 40
	ti.SetValue("")
	ti.Focus()
	m.addNameInput = ti
}

func (m *configuratorSessionModel) initJSONInput() {
	ta := textarea.New()
	ta.Prompt = "> "
	ta.Placeholder = "Paste it here"
	ta.SetWidth(80)
	ta.SetHeight(10)
	ta.ShowLineNumbers = true
	ta.SetValue("")
	ta.Focus()
	m.addJSONInput = ta
}

func (m *configuratorSessionModel) adjustInputsToViewport() {
	if m.width <= 0 {
		return
	}
	contentWidth := contentWidthForTerminal(m.width)
	available := maxInt(1, contentWidth-inputContainerStyle().GetHorizontalFrameSize())
	m.addNameInput.Width = minInt(40, available)
	m.addJSONInput.SetWidth(minInt(80, available))
	if m.height > 14 {
		m.addJSONInput.SetHeight(m.height - 18)
	}
}

func (m configuratorSessionModel) renderSelectionScreen(
	title string,
	subtitle string,
	options []string,
	cursor int,
	hint string,
) string {
	styles := resolveUIStyles(CurrentUIPreferences())
	contentWidth := 0
	if m.width > 0 {
		contentWidth = contentWidthForTerminal(m.width)
	}

	body := renderSelectableRows(options, cursor, contentWidth, styles)
	return renderScreen(
		m.width,
		m.height,
		title,
		subtitle,
		body,
		hint,
	)
}

func (m configuratorSessionModel) inputContainerWidth() int {
	if m.width > 0 {
		return maxInt(1, contentWidthForTerminal(m.width))
	}
	return 40 + inputContainerStyle().GetHorizontalFrameSize()
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

func serverPeerOptionLabel(peer serverConfiguration.AllowedPeer) string {
	status := "disabled"
	if peer.Enabled {
		status = "enabled"
	}
	name := strings.TrimSpace(peer.Name)
	if name == "" {
		name = fmt.Sprintf("client-%d", peer.ClientID)
	}
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
