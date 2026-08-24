package bubble_tea

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"tungo/internal/config"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m Configurator) updateClientSelectScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		if len(m.modeOptions) == 1 {
			m.resultErr = ErrConfiguratorUserExit
			m.done = true
			return m, tea.Quit
		}
		m.screen = configuratorScreenMode
		return m, nil
	}

	m.updateCursor(msg, len(m.client.menuOptions))
	if msg.String() != "enter" || len(m.client.menuOptions) == 0 {
		return m, nil
	}

	selected := m.client.menuOptions[m.cursor]
	switch selected {
	case clientAddLabel:
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientAddName
		m.initNameInput()
		m.adjustInputsToViewport()
		return m, textinput.Blink
	case clientRemoveLabel:
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
		if err := m.options.ClientConfigurationControl.Select(selected); err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}

		_, cfgErr := m.options.ClientConfigurationControl.RuntimeInfo()
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

		m = m.startModeWithDaemonGuard(config.ModeClient, configuratorScreenClientSelect, false)
		if m.done {
			m = m.persistAutoSelectClientConfig(selected)
			return m, tea.Quit
		}
		if isDaemonStartConfirmationScreen(m.screen) {
			m.pendingClientConfig = selected
		}
		return m, nil
	}
}

func (m Configurator) updateClientRemoveScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	if err := m.options.ClientConfigurationControl.Delete(toDelete); err != nil {
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

func (m Configurator) updateClientAddNameScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m Configurator) updateClientAddJSONScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

		if err := m.options.ClientConfigurationControl.CreateFromJSON(m.client.addName, m.client.addJSONInput.Value()); err != nil {
			if isInvalidClientConfigurationError(err) {
				m.client.invalidErr = err
				m.client.invalidConfig = ""
				m.client.invalidAllowDelete = false
				m.cursor = 0
				m.screen = configuratorScreenClientInvalid
				return m, nil
			}
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

func (m Configurator) updateClientInvalidScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.notice = ""
		m.cursor = 0
		m.screen = configuratorScreenClientSelect
		return m, nil
	}

	options := []string{invalidOKLabel}
	if m.client.invalidAllowDelete {
		options = []string{invalidDeleteLabel, invalidOKLabel}
	}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" || len(options) == 0 {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == invalidDeleteLabel && m.client.invalidAllowDelete {
		if strings.TrimSpace(m.client.invalidConfig) == "" {
			m.resultErr = errors.New("invalid configuration cannot be deleted")
			m.done = true
			return m, tea.Quit
		}
		if err := m.options.ClientConfigurationControl.Delete(m.client.invalidConfig); err != nil {
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

func (m Configurator) persistAutoSelectClientConfig(selected string) Configurator {
	if strings.TrimSpace(selected) == "" {
		return m
	}
	p := m.settings.Current()
	p.AutoSelectClientConfig = selected
	m.settings.update(p)
	_ = savePreferencesToDisk(p)
	return m
}

func (m *Configurator) reloadClientConfigs() error {
	configs, err := m.options.ClientConfigurationControl.List()
	if err != nil {
		return err
	}
	m.client.configs = configs
	m.client.menuOptions = make([]string, 0, len(configs)+3)
	m.client.menuOptions = append(m.client.menuOptions, configs...)
	if len(configs) > 0 {
		m.client.menuOptions = append(m.client.menuOptions, clientRemoveLabel)
	}
	m.client.menuOptions = append(m.client.menuOptions, clientAddLabel)
	return nil
}

func (m *Configurator) initNameInput() {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Give it a name"
	ti.CharLimit = 256
	ti.SetWidth(40)
	ti.SetValue("")
	ti.Focus()
	m.client.addNameInput = ti
}

func (m *Configurator) tryFormatJSON() {
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

func (m *Configurator) initJSONInput() {
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

func (m *Configurator) adjustInputsToViewport() {
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
	runes := []rune(message)
	if len(runes) > 120 {
		return string(runes[:117]) + "..."
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
