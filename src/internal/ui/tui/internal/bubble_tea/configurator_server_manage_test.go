package bubble_tea

import (
	"errors"
	"strings"
	"testing"

	serverconfig "tungo/internal/config/server"

	tea "charm.land/bubbletea/v2"
)

func newSessionModelForServerManageTests(
	t *testing.T,
	control *testConfigurationControl,
) Configurator {
	t.Helper()
	model, err := NewConfigurator(testConfiguratorOptions(control), testSettings())
	if err != nil {
		t.Fatalf("NewConfigurator error: %v", err)
	}
	model.screen = configuratorScreenServerManage
	model.server.managePeers = append([]serverconfig.AllowedPeer(nil), control.peers...)
	model.server.manageLabels = buildServerManageLabels(model.server.managePeers)
	model.cursor = 0
	return model
}

func keyRunes(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyNamed(k rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: k}
}

func TestServerManage_DeleteFlow_ConfirmRemovesPeer(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
			{Name: "beta", ClientID: 2, Enabled: false},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state, ok := nextModel.(Configurator)
	if !ok {
		t.Fatalf("unexpected model type: %T", nextModel)
	}
	if state.screen != configuratorScreenServerDeleteConfirm {
		t.Fatalf("expected delete confirm screen, got %v", state.screen)
	}

	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state, ok = nextModel.(Configurator)
	if !ok {
		t.Fatalf("unexpected model type after confirm: %T", nextModel)
	}
	if manager.removeCalls != 1 || manager.lastRemoved != 1 {
		t.Fatalf("expected one removal for client 1, calls=%d last=%d", manager.removeCalls, manager.lastRemoved)
	}
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected return to manage screen, got %v", state.screen)
	}
	if len(state.server.managePeers) != 1 || state.server.managePeers[0].ClientID != 2 {
		t.Fatalf("unexpected peers after delete: %+v", state.server.managePeers)
	}
	if !strings.Contains(state.notice, "removed") {
		t.Fatalf("expected removal notice, got %q", state.notice)
	}
}

func TestServerManage_DeleteFlow_CancelKeepsPeer(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 10, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(Configurator)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyDown))
	state = nextModel.(Configurator)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state = nextModel.(Configurator)

	if manager.removeCalls != 0 {
		t.Fatalf("expected no removal call on cancel, got %d", manager.removeCalls)
	}
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected return to manage screen, got %v", state.screen)
	}
	if len(state.server.managePeers) != 1 || state.server.managePeers[0].ClientID != 10 {
		t.Fatalf("unexpected peers after cancel: %+v", state.server.managePeers)
	}
}

func TestServerManage_DeleteFlow_LastPeerReturnsToServerMenu(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "solo", ClientID: 99, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(Configurator)
	nextModel, _ = state.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state = nextModel.(Configurator)

	if state.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select when list is empty, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "No clients configured yet.") {
		t.Fatalf("expected empty-list notice, got %q", state.notice)
	}
}

func TestServerManage_ToggleEnabled_Error_ShowsNotice(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
		setEnabledErr: errors.New("enable failed"),
	}
	model := newSessionModelForServerManageTests(t, manager)

	nextModel, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(Configurator)

	if state.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select on error, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "Failed to update client #1") {
		t.Fatalf("expected error notice, got %q", state.notice)
	}
}

func TestServerManage_ToggleEnabled_ListError_Exits(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	// After SetPeerEnabled succeeds, make ListPeers fail.
	manager.listPeersErr = errors.New("list failed")

	nextModel, cmd := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(Configurator)

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

func TestServerManage_DeleteNoEmptyPeers(t *testing.T) {
	manager := &testConfigurationControl{
		peers: nil,
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.server.managePeers = nil
	model.server.manageLabels = nil

	// 'd' with no peers should be a no-op.
	nextModel, _ := model.updateServerManageScreen(keyRunes('d'))
	state := nextModel.(Configurator)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected to stay on manage screen, got %v", state.screen)
	}
}

func TestServerDeleteConfirm_EscRestoresCursorNoPeers(t *testing.T) {
	manager := &testConfigurationControl{
		peers: nil,
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.server.managePeers = nil
	model.server.deleteCursor = 0

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEsc))
	state := nextModel.(Configurator)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if state.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", state.cursor)
	}
}

func TestServerDeleteConfirm_RemoveError_ShowsNotice(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
		removeErr: errors.New("remove failed"),
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.server.deletePeer = manager.peers[0]
	model.cursor = 0

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(Configurator)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if !strings.Contains(state.notice, "Failed to remove client #1") {
		t.Fatalf("expected removal error notice, got %q", state.notice)
	}
}

func TestServerDeleteConfirm_ListError_Exits(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.server.deletePeer = manager.peers[0]
	model.cursor = 0
	// After remove, make list fail.
	// The remove will succeed (removeErr is nil), and the peer is removed from slice.
	// Then ListPeers will be called. Need to make it fail after remove.
	// Since our stub checks listErr, set it before the call.
	manager.listPeersErr = errors.New("list failed after delete")

	nextModel, cmd := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(Configurator)
	if !state.done {
		t.Fatal("expected done=true on list error after delete")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestServerDeleteConfirm_CancelWithPeers_RestoresCursor(t *testing.T) {
	manager := &testConfigurationControl{
		peers: []serverconfig.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
			{Name: "beta", ClientID: 2, Enabled: false},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.screen = configuratorScreenServerDeleteConfirm
	model.server.deletePeer = manager.peers[1]
	model.server.deleteCursor = 1
	model.cursor = 1 // cursor on "Cancel"

	nextModel, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	state := nextModel.(Configurator)
	if state.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", state.screen)
	}
	if state.cursor != 1 {
		t.Fatalf("expected cursor restored to 1, got %d", state.cursor)
	}
}
