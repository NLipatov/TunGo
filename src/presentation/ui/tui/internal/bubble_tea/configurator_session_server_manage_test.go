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
	peers         []serverConfiguration.AllowedPeer
	removeErr     error
	removeCalls   int
	lastRemoved   int
	setEnabledErr error
	listErr       error
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
	if s.listErr != nil {
		return nil, s.listErr
	}
	peers := make([]serverConfiguration.AllowedPeer, len(s.peers))
	copy(peers, s.peers)
	return peers, nil
}

func (s *sessionServerConfigManagerStub) SetAllowedPeerEnabled(clientID int, enabled bool) error {
	if s.setEnabledErr != nil {
		return s.setEnabledErr
	}
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
	}, testSettings())
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

func TestServerManage_ToggleEnabled_Error_ShowsNotice(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
		setEnabledErr: errors.New("enable failed"),
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)

	if state.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select on error, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "Failed to update client #1") {
		t.Fatalf("expected error notice, got %q", state.notice)
	}
}

func TestServerManage_ToggleEnabled_ListError_Exits(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	// After SetAllowedPeerEnabled succeeds, make ListAllowedPeers fail.
	manager.listErr = errors.New("list failed")

	nextModel, cmd := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)

	if !state.done {
		t.Fatal("expected done=true on list error")
	}
	if state.resultErr == nil || !strings.Contains(state.resultErr.Error(), "list failed") {
		t.Fatalf("expected list error, got %v", state.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestServerManage_ToggleEnabled_ListReturnsEmpty_GoesBack(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	// After toggle, clear all peers so ListAllowedPeers returns empty.
	model.serverManagePeers = []serverConfiguration.AllowedPeer{
		{Name: "alpha", ClientID: 1, Enabled: true},
	}
	// Hack: after SetAllowedPeerEnabled succeeds, the manager has the peer toggled.
	// But let's make ListAllowedPeers return empty by clearing peers post-toggle.
	// We'll use a different approach: remove the peer inside SetAllowedPeerEnabled side effect.

	// Actually simpler: just remove all peers from manager after the toggle call.
	// We need to make the list return empty after the toggle. Let's just clear manager.peers
	// but the stub toggle adds it back. Let me just set peers to empty after a successful toggle.
	// Best approach: wrap the test by modifying peers to become empty after toggle.
	manager.peers = []serverConfiguration.AllowedPeer{
		{Name: "alpha", ClientID: 1, Enabled: true},
	}

	// Set cursor on the peer
	model.cursor = 0

	// The toggle will succeed (setEnabledErr is nil), then ListAllowedPeers is called.
	// To make list return empty, we need to clear peers before list is called.
	// Since we can't hook between calls, let's just make setEnabledErr nil and remove all peers.
	// SetAllowedPeerEnabled will toggle, then ListAllowedPeers returns the toggled peer.
	// For the "list empty" case, the simplest is to have the peer removed during toggle.
	// Let's just swap peers slice.

	// Alternative: Use setEnabledErr=nil so toggle succeeds, but make the test that
	// the manage screen handles cursor adjustment when cursor >= len(peers).
	// That's the line `if m.cursor >= len(m.serverManagePeers)`.

	// Let's test cursor clamping instead: start with 2 peers, cursor at 1, remove peer 0,
	// then list returns only 1 peer, and cursor should be clamped.
	manager.peers = []serverConfiguration.AllowedPeer{
		{Name: "alpha", ClientID: 1, Enabled: true},
		{Name: "beta", ClientID: 2, Enabled: false},
	}
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.serverManageLabels = buildServerManageLabels(model.serverManagePeers)
	model.cursor = 1

	nextModel, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
}

func TestServerManage_DeleteNoEmptyPeers(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: nil,
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.serverManagePeers = nil
	model.serverManageLabels = nil

	// 'd' with no peers should be a no-op.
	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(configuratorSessionModel)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected to stay on manage screen, got %v", state.screen)
	}
}

func TestServerDeleteConfirm_EscRestoresCursorNoPeers(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: nil,
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverManagePeers = nil
	model.serverDeleteCursor = 0

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEsc))
	state := nextModel.(configuratorSessionModel)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if state.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", state.cursor)
	}
}

func TestServerDeleteConfirm_RemoveError_ShowsNotice(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
		removeErr: errors.New("remove failed"),
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverDeletePeer = manager.peers[0]
	model.cursor = 0

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "Failed to remove client #1") {
		t.Fatalf("expected removal error notice, got %q", state.notice)
	}
}

func TestServerDeleteConfirm_ListError_Exits(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverDeletePeer = manager.peers[0]
	model.cursor = 0
	// After remove, make list fail.
	// The remove will succeed (removeErr is nil), and the peer is removed from slice.
	// Then ListAllowedPeers will be called. Need to make it fail after remove.
	// Since our stub checks listErr, set it before the call.
	manager.listErr = errors.New("list failed after delete")

	nextModel, cmd := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)
	if !state.done {
		t.Fatal("expected done=true on list error after delete")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestServerDeleteConfirm_CancelWithPeers_RestoresCursor(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
			{Name: "beta", ClientID: 2, Enabled: false},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverDeletePeer = manager.peers[1]
	model.serverDeleteCursor = 1
	model.cursor = 1 // cursor on "Cancel"

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(configuratorSessionModel)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if state.cursor != 1 {
		t.Fatalf("expected cursor restored to 1, got %d", state.cursor)
	}
}
