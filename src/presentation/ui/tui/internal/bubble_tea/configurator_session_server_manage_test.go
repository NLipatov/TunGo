package bubble_tea

import (
	"errors"
	"strings"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"

	tea "github.com/charmbracelet/bubbletea"
)

type sessionObserverStub struct{}

func (sessionObserverStub) Observe() ([]string, error) { return nil, nil }

type sessionSelectorStub struct{}

func (sessionSelectorStub) Select(string) error { return nil }

type sessionCreatorStub struct{}

func (sessionCreatorStub) Create(clientConfiguration.Configuration, string) error { return nil }

type sessionDeleterStub struct{}

func (sessionDeleterStub) Delete(string) error { return nil }

type sessionClientConfigManagerStub struct{}

func (sessionClientConfigManagerStub) Configuration() (*clientConfiguration.Configuration, error) {
	return nil, nil
}

type sessionServerConfigManagerStub struct {
	peers       []serverConfiguration.AllowedPeer
	removeErr   error
	removeCalls int
	lastRemoved int
}

func (s *sessionServerConfigManagerStub) Configuration() (*serverConfiguration.Configuration, error) {
	return &serverConfiguration.Configuration{AllowedPeers: append([]serverConfiguration.AllowedPeer(nil), s.peers...)}, nil
}

func (s *sessionServerConfigManagerStub) IncrementClientCounter() error { return nil }

func (s *sessionServerConfigManagerStub) InjectX25519Keys(_, _ []byte) error { return nil }

func (s *sessionServerConfigManagerStub) AddAllowedPeer(peer serverConfiguration.AllowedPeer) error {
	s.peers = append(s.peers, peer)
	return nil
}

func (s *sessionServerConfigManagerStub) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	peers := make([]serverConfiguration.AllowedPeer, len(s.peers))
	copy(peers, s.peers)
	return peers, nil
}

func (s *sessionServerConfigManagerStub) SetAllowedPeerEnabled(clientID int, enabled bool) error {
	for i := range s.peers {
		if s.peers[i].ClientID == clientID {
			s.peers[i].Enabled = enabled
			return nil
		}
	}
	return nil
}

func (s *sessionServerConfigManagerStub) RemoveAllowedPeer(clientID int) error {
	s.removeCalls++
	s.lastRemoved = clientID
	if s.removeErr != nil {
		return s.removeErr
	}
	for i := range s.peers {
		if s.peers[i].ClientID == clientID {
			s.peers = append(s.peers[:i], s.peers[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (s *sessionServerConfigManagerStub) EnsureIPv6Subnets() error { return nil }

func (s *sessionServerConfigManagerStub) InvalidateCache() {}

func newSessionModelForServerManageTests(
	t *testing.T,
	manager *sessionServerConfigManagerStub,
) configuratorSessionModel {
	t.Helper()
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	})
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerManage
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.serverManageLabels = buildServerManageLabels(model.serverManagePeers)
	model.cursor = 0
	return model
}

func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func keyNamed(k tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: k}
}

func TestServerManage_DeleteFlow_ConfirmRemovesPeer(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
			{Name: "beta", ClientID: 2, Enabled: false},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state, ok := nextModel.(configuratorSessionModel)
	if !ok {
		t.Fatalf("unexpected model type: %T", nextModel)
	}
	if state.screen != configuratorScreenServerDeleteConfirm {
		t.Fatalf("expected delete confirm screen, got %v", state.screen)
	}

	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state, ok = nextModel.(configuratorSessionModel)
	if !ok {
		t.Fatalf("unexpected model type after confirm: %T", nextModel)
	}
	if manager.removeCalls != 1 || manager.lastRemoved != 1 {
		t.Fatalf("expected one removal for client 1, calls=%d last=%d", manager.removeCalls, manager.lastRemoved)
	}
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected return to manage screen, got %v", state.screen)
	}
	if len(state.serverManagePeers) != 1 || state.serverManagePeers[0].ClientID != 2 {
		t.Fatalf("unexpected peers after delete: %+v", state.serverManagePeers)
	}
	if !strings.Contains(state.notice, "removed") {
		t.Fatalf("expected removal notice, got %q", state.notice)
	}
}

func TestServerManage_DeleteFlow_CancelKeepsPeer(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 10, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(configuratorSessionModel)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyDown))
	state = nextModel.(configuratorSessionModel)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state = nextModel.(configuratorSessionModel)

	if manager.removeCalls != 0 {
		t.Fatalf("expected no removal call on cancel, got %d", manager.removeCalls)
	}
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected return to manage screen, got %v", state.screen)
	}
	if len(state.serverManagePeers) != 1 || state.serverManagePeers[0].ClientID != 10 {
		t.Fatalf("unexpected peers after cancel: %+v", state.serverManagePeers)
	}
}

func TestServerManage_DeleteFlow_LastPeerReturnsToServerMenu(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "solo", ClientID: 99, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(configuratorSessionModel)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state = nextModel.(configuratorSessionModel)

	if state.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select when list is empty, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "No clients configured yet.") {
		t.Fatalf("expected empty-list notice, got %q", state.notice)
	}
}
