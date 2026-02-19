package bubble_tea

import (
	"context"
	"strings"
	"testing"

	"tungo/domain/mode"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"

	tea "github.com/charmbracelet/bubbletea"
)

func defaultUnifiedConfigOpts() ConfiguratorSessionOptions {
	return ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "test", ClientID: 1, Enabled: true},
			},
		},
	}
}

func newTestUnifiedModel(t *testing.T) (unifiedSessionModel, chan unifiedEvent) {
	t.Helper()
	events := make(chan unifiedEvent, 8)
	model, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events)
	if err != nil {
		t.Fatalf("newUnifiedSessionModel: %v", err)
	}
	return model, events
}

// --- Phase: configuring ---

func TestUnifiedSession_InitialPhaseIsConfiguring(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring, got %d", m.phase)
	}
}

func TestUnifiedSession_ConfiguratorViewShown(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.width = 100
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Select mode") {
		t.Fatalf("expected configurator view, got: %q", view)
	}
}

func TestUnifiedSession_WindowSizePropagated(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := result.(unifiedSessionModel)
	if updated.width != 120 || updated.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", updated.width, updated.height)
	}
}

// --- Transition: configuring -> waitingForRuntime ---

func TestUnifiedSession_ModeSelection_TransitionsToWaiting(t *testing.T) {
	m, events := newTestUnifiedModel(t)

	// Navigate to server mode and select "start server".
	m.configurator.screen = configuratorScreenServerSelect
	m.configurator.cursor = 0 // "start server"
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(unifiedSessionModel)

	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", updated.phase)
	}

	// Check event was sent.
	select {
	case event := <-events:
		if event.kind != unifiedEventModeSelected {
			t.Fatalf("expected unifiedEventModeSelected, got %d", event.kind)
		}
		if event.mode != mode.Server {
			t.Fatalf("expected Server mode, got %v", event.mode)
		}
	default:
		t.Fatal("expected mode selected event")
	}
}

// --- Transition: waitingForRuntime -> runtime ---

func TestUnifiedSession_ActivateRuntime_TransitionsToRuntime(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseWaitingForRuntime
	m.width = 100
	m.height = 30

	result, cmd := m.Update(activateRuntimeMsg{
		ctx: context.Background(),
		options: RuntimeDashboardOptions{
			Mode: RuntimeDashboardServer,
		},
	})
	updated := result.(unifiedSessionModel)

	if updated.phase != phaseRuntime {
		t.Fatalf("expected phaseRuntime, got %d", updated.phase)
	}
	if updated.runtime == nil {
		t.Fatal("expected runtime to be set")
	}
	if updated.runtime.mode != RuntimeDashboardServer {
		t.Fatalf("expected server mode, got %q", updated.runtime.mode)
	}
	if updated.runtime.width != 100 || updated.runtime.height != 30 {
		t.Fatalf("expected propagated size 100x30, got %dx%d", updated.runtime.width, updated.runtime.height)
	}
	if cmd == nil {
		t.Fatal("expected init cmd from runtime")
	}
}

func TestUnifiedSession_ActivateRuntime_IgnoredInWrongPhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	// phase is phaseConfiguring, not phaseWaitingForRuntime
	result, cmd := m.Update(activateRuntimeMsg{
		ctx:     context.Background(),
		options: RuntimeDashboardOptions{},
	})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when ActivateRuntime in wrong phase")
	}
}

// --- Waiting phase view ---

func TestUnifiedSession_WaitingPhase_ShowsStarting(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseWaitingForRuntime
	m.width = 100
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Starting...") {
		t.Fatalf("expected waiting view with Starting..., got: %q", view)
	}
}

// --- Runtime phase: exit ---

func TestUnifiedSession_RuntimeExit_QuitsProgram(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.runtime = &rt

	// Simulate q key press.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := result.(unifiedSessionModel)
	_ = updated

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event")
	}
}

// --- Runtime phase: reconfigure ---

func TestUnifiedSession_RuntimeReconfigure_TransitionsToConfiguring(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.width = 100
	m.height = 30
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.runtime = &rt

	// Simulate esc -> confirm reconfigure.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(unifiedSessionModel)
	// Move cursor to "Stop tunnel and reconfigure" (index 1).
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated = result.(unifiedSessionModel)
	// Confirm.
	result, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = result.(unifiedSessionModel)

	if updated.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring after reconfigure, got %d", updated.phase)
	}
	if updated.runtime != nil {
		t.Fatal("expected runtime to be cleared")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventReconfigure {
			t.Fatalf("expected unifiedEventReconfigure, got %d", event.kind)
		}
	default:
		t.Fatal("expected reconfigure event")
	}
}

// --- Configurator quit sends exit event ---

func TestUnifiedSession_ConfiguratorQuit_SendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)

	// Press q to quit from configurator.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	_ = result

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on configurator quit")
	}
}

// --- Context done ---

func TestUnifiedSession_ContextDone_SendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)

	result, cmd := m.Update(contextDoneMsg{})
	_ = result

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on context done")
	}
}

// --- RuntimeContextDone in runtime phase ---

func TestUnifiedSession_RuntimeContextDone_SendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.runtime = &rt

	result, cmd := m.Update(runtimeContextDoneMsg{})
	_ = result

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on runtime context done")
	}
}

func TestUnifiedSession_RuntimeContextDone_IgnoredOutsideRuntimePhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	// phase is phaseConfiguring
	result, cmd := m.Update(runtimeContextDoneMsg{})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when runtimeContextDone not in runtime phase")
	}
}

// --- filterQuit ---

func TestFilterQuit_NilCmd(t *testing.T) {
	result := filterQuit(nil)
	if result != nil {
		t.Fatal("expected nil")
	}
}

func TestFilterQuit_QuitMsgFiltered(t *testing.T) {
	cmd := func() tea.Msg { return tea.QuitMsg{} }
	result := filterQuit(cmd)
	if result != nil {
		t.Fatal("expected tea.Quit to be filtered out")
	}
}

func TestFilterQuit_NonQuitMsgPreserved(t *testing.T) {
	original := tea.WindowSizeMsg{Width: 80, Height: 24}
	cmd := func() tea.Msg { return original }
	result := filterQuit(cmd)
	if result == nil {
		t.Fatal("expected cmd to be preserved")
	}
	msg := result()
	if _, ok := msg.(tea.WindowSizeMsg); !ok {
		t.Fatalf("expected WindowSizeMsg, got %T", msg)
	}
}

// --- WaitingForRuntime ignores messages ---

func TestUnifiedSession_WaitingPhase_IgnoresKeyMessages(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseWaitingForRuntime

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd in waiting phase")
	}
}

// --- Runtime phase view ---

func TestUnifiedSession_RuntimePhase_ShowsRuntimeView(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.width = 100
	m.height = 30
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		Mode: RuntimeDashboardServer,
	})
	rt.width = 100
	rt.height = 30
	m.runtime = &rt

	view := m.View()
	if !strings.Contains(view, "Mode: Server") {
		t.Fatalf("expected runtime view with server mode, got: %q", view)
	}
}
