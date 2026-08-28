package bubble_tea

import (
	"fmt"
	"strings"

	serverconfig "tungo/internal/config/server"
	"tungo/internal/mode"

	tea "charm.land/bubbletea/v2"
)

func (m Configurator) updateServerSelectScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case serverStartLabel:
		m = m.startModeWithDaemonGuard(mode.Server, configuratorScreenServerSelect, false)
		if m.done {
			return m, tea.Quit
		}
		return m, nil
	case serverAddLabel:
		generated, err := m.options.ServerConfigurations.GenerateClient()
		if err != nil {
			m.resultErr = err
			m.done = true
			return m, tea.Quit
		}
		m.notice = fmt.Sprintf("Client configuration saved to %s", generated.Path)
		return m, nil
	case serverManageLabel:
		peers, err := m.options.ServerConfigurations.Peers()
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

func (m Configurator) updateServerManageScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	if err := m.options.ServerConfigurations.SetPeerEnabled(peer.ClientID, nextEnabled); err != nil {
		m.notice = fmt.Sprintf("Failed to update client #%d: %v", peer.ClientID, err)
		m.screen = configuratorScreenServerSelect
		m.cursor = 0
		return m, nil
	}

	peers, err := m.options.ServerConfigurations.Peers()
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

func (m Configurator) updateServerDeleteConfirmScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

	options := []string{serverDeleteConfirmLabel, cancelLabel}
	m.updateCursor(msg, len(options))
	if msg.String() != "enter" {
		return m, nil
	}

	selected := options[m.cursor]
	if selected == cancelLabel {
		if len(m.server.managePeers) > 0 {
			m.cursor = minInt(m.server.deleteCursor, len(m.server.managePeers)-1)
		} else {
			m.cursor = 0
		}
		m.screen = configuratorScreenServerManage
		return m, nil
	}

	if err := m.options.ServerConfigurations.RemovePeer(m.server.deletePeer.ClientID); err != nil {
		m.notice = fmt.Sprintf("Failed to remove client #%d: %v", m.server.deletePeer.ClientID, err)
		m.screen = configuratorScreenServerManage
		m.cursor = 0
		return m, nil
	}

	peers, err := m.options.ServerConfigurations.Peers()
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

// buildServerManageLabels formats allowed peers as labels for the server management screen.
func buildServerManageLabels(peers []serverconfig.AllowedPeer) []string {
	labels := make([]string, 0, len(peers))
	for _, peer := range peers {
		labels = append(labels, serverPeerOptionLabel(peer))
	}
	return labels
}

// serverPeerDisplayName returns the peer's trimmed name, or a client identifier when the name is empty.
func serverPeerDisplayName(peer serverconfig.AllowedPeer) string {
	name := strings.TrimSpace(peer.Name)
	if name == "" {
		return fmt.Sprintf("client-%d", peer.ClientID)
	}
	return name
}

// serverPeerOptionLabel formats a peer's client ID, display name, and enabled status for presentation.
func serverPeerOptionLabel(peer serverconfig.AllowedPeer) string {
	status := "disabled"
	if peer.Enabled {
		status = "enabled"
	}
	name := serverPeerDisplayName(peer)
	return fmt.Sprintf("#%d %s [%s]", peer.ClientID, name, status)
}
