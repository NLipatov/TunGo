package bubble_tea

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"tungo/domain/mode"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"

	tea "charm.land/bubbletea/v2"
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
		ServerSupported: true,
	}
}

func newTestUnifiedModel(t *testing.T) (unifiedSessionModel, chan unifiedEvent) {
	t.Helper()
	events := make(chan unifiedEvent, 8)
	model, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events, testSettings())
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
	view := m.View().Content
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
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	if updated.runtimeSeq != 1 {
		t.Fatalf("expected runtimeSeq=1, got %d", updated.runtimeSeq)
	}
	if updated.runtime.runtimeSeq != 1 {
		t.Fatalf("expected runtime.runtimeSeq=1, got %d", updated.runtime.runtimeSeq)
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
	view := m.View().Content
	if !strings.Contains(view, "Starting...") {
		t.Fatalf("expected waiting view with Starting..., got: %q", view)
	}
}

// --- Runtime phase: exit ---

func TestUnifiedSession_RuntimeExit_QuitsProgram(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	m.runtime = &rt

	// Simulate ctrl+c key press.
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
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
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	m.runtime = &rt

	// Simulate esc -> confirm reconfigure.
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := result.(unifiedSessionModel)
	// Move cursor to "Stop" (index 1).
	result, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated = result.(unifiedSessionModel)
	// Confirm.
	result, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	// Press ctrl+c to quit from configurator.
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
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

func TestUnifiedSession_RuntimeContextDone_SendsDisconnectedEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtimeSeq = 3
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.runtimeSeq = 3
	m.runtime = &rt

	result, cmd := m.Update(runtimeContextDoneMsg{seq: 3})
	updated := result.(unifiedSessionModel)

	if cmd != nil {
		t.Fatal("expected nil cmd (no tea.Quit on runtime disconnect)")
	}
	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", updated.phase)
	}
	if updated.runtime != nil {
		t.Fatal("expected runtime to be nil after disconnect")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventRuntimeDisconnected {
			t.Fatalf("expected unifiedEventRuntimeDisconnected, got %d", event.kind)
		}
	default:
		t.Fatal("expected disconnected event on runtime context done")
	}
}

func TestUnifiedSession_RuntimeContextDone_StaleSeqIgnored(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtimeSeq = 5
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.runtimeSeq = 5
	m.runtime = &rt

	// Stale message from a previous runtime (seq=2).
	result, cmd := m.Update(runtimeContextDoneMsg{seq: 2})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseRuntime {
		t.Fatalf("expected phaseRuntime (stale msg ignored), got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for stale runtimeContextDoneMsg")
	}
	select {
	case event := <-events:
		t.Fatalf("expected no event for stale msg, got %d", event.kind)
	default:
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

// --- stopAllLogWaits ---

func TestUnifiedSession_ContextDone_StopsLogWaits(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtimeSeq = 1
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.runtimeSeq = 1
	rtLogStop := make(chan struct{})
	rt.logWaitStop = rtLogStop
	m.runtime = &rt
	cfgLogStop := make(chan struct{})
	m.configurator.logWaitStop = cfgLogStop

	m.Update(contextDoneMsg{})

	// Verify both channels were closed (reading from closed channel returns immediately).
	select {
	case <-cfgLogStop:
	default:
		t.Fatal("expected configurator logWaitStop to be closed")
	}
	select {
	case <-rtLogStop:
	default:
		t.Fatal("expected runtime logWaitStop to be closed")
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
	if result == nil {
		t.Fatal("expected non-nil wrapper cmd")
	}
	msg := result()
	if msg != nil {
		t.Fatalf("expected nil msg (QuitMsg filtered), got %T", msg)
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

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd in waiting phase")
	}
}

// --- Runtime phase view ---

func TestUnifiedSession_RuntimePhase_NilRuntime_ShowsWaitingView(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtime = nil
	m.width = 100
	m.height = 30
	view := m.View().Content
	if !strings.Contains(view, "Starting...") {
		t.Fatalf("expected waiting view when runtime is nil, got: %q", view)
	}
}

func TestUnifiedSession_View_DefaultPhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = unifiedPhase(99)
	view := m.View().Content
	if view != "" {
		t.Fatalf("expected empty view for unknown phase, got: %q", view)
	}
}

func TestUnifiedSession_DelegateToActive_DefaultPhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = unifiedPhase(99)
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := result.(unifiedSessionModel)
	if updated.phase != unifiedPhase(99) {
		t.Fatalf("expected phase unchanged, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for unknown phase")
	}
}

func TestFilterQuit_BatchMsg(t *testing.T) {
	quitCmd := func() tea.Msg { return tea.QuitMsg{} }
	normalMsg := tea.WindowSizeMsg{Width: 80, Height: 24}
	normalCmd := func() tea.Msg { return normalMsg }

	batchCmd := func() tea.Msg {
		return tea.BatchMsg{quitCmd, normalCmd}
	}
	result := filterQuit(batchCmd)
	if result == nil {
		t.Fatal("expected non-nil wrapper cmd")
	}
	msg := result()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected BatchMsg, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 sub-commands in filtered batch, got %d", len(batch))
	}
	// First sub-cmd (originally QuitMsg) should be filtered to nil.
	if batch[0]() != nil {
		t.Fatal("expected QuitMsg sub-cmd to be filtered to nil")
	}
	// Second sub-cmd should pass through.
	if _, ok := batch[1]().(tea.WindowSizeMsg); !ok {
		t.Fatalf("expected WindowSizeMsg to pass through, got %T", batch[1]())
	}
}

func TestNewUnifiedSessionModel_ErrorPath(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	// Missing required dependencies → newConfiguratorSessionModel fails.
	_, err := newUnifiedSessionModel(context.Background(), ConfiguratorSessionOptions{}, events, testSettings())
	if err == nil {
		t.Fatal("expected error when ConfiguratorSessionOptions are empty")
	}
}

func TestWaitForContextDone_NilContext(t *testing.T) {
	cmd := waitForContextDone(nil)
	if cmd != nil {
		t.Fatal("expected nil cmd for nil context")
	}
}

func TestWaitForContextDone_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := waitForContextDone(ctx)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(contextDoneMsg); !ok {
		t.Fatalf("expected contextDoneMsg, got %T", msg)
	}
}

func TestUnifiedSession_UpdateConfigurator_NonUserExitError(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	// Simulate esc in mode screen → sets ErrConfiguratorSessionUserExit.
	// For non-user-exit error, we need a different approach.
	// Set configurator done with a non-exit error directly.
	m.configurator.done = true
	m.configurator.resultErr = errors.New("unexpected error")

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	_ = result.(unifiedSessionModel)
	if cmd == nil {
		t.Fatal("expected quit cmd for configurator error")
	}
	select {
	case event := <-events:
		if event.kind != unifiedEventError {
			t.Fatalf("expected unifiedEventError, got %d", event.kind)
		}
	default:
		t.Fatal("expected error event")
	}
}

func TestUnifiedSession_UpdateRuntime_NilRuntime(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtime = nil

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseRuntime {
		t.Fatalf("expected phaseRuntime, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when runtime is nil")
	}
}

func TestUnifiedSessionModel_Init_ReturnsBatchCmd(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected non-nil init batch cmd")
	}
}

func TestUnifiedSession_UpdateRuntime_ReconfigureRequested(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.reconfigureRequested = true
	m.runtime = &rt

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring after reconfigure, got %d", updated.phase)
	}
	if updated.runtime != nil {
		t.Fatal("expected runtime to be nil after reconfigure")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for reconfigure (batch of configurator Init + ClearScreen)")
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

func TestUnifiedSession_UpdateRuntime_ExitRequested(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.exitRequested = true
	m.runtime = &rt

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	_ = result.(unifiedSessionModel)
	if cmd == nil {
		t.Fatal("expected quit cmd for exit")
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

func TestUnifiedSession_ActivateRuntimeMsg(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseWaitingForRuntime
	m.width = 100
	m.height = 30

	result, cmd := m.Update(activateRuntimeMsg{
		ctx:     context.Background(),
		options: RuntimeDashboardOptions{Mode: RuntimeDashboardServer},
	})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseRuntime {
		t.Fatalf("expected phaseRuntime, got %d", updated.phase)
	}
	if updated.runtime == nil {
		t.Fatal("expected non-nil runtime after activation")
	}
	if updated.runtime.width != 100 || updated.runtime.height != 30 {
		t.Fatal("expected runtime to inherit session dimensions")
	}
	if cmd == nil {
		t.Fatal("expected non-nil init cmd from runtime")
	}
}

func TestUnifiedSession_ActivateRuntimeMsg_IgnoredInWrongPhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseConfiguring

	result, cmd := m.Update(activateRuntimeMsg{
		ctx:     context.Background(),
		options: RuntimeDashboardOptions{},
	})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseConfiguring {
		t.Fatalf("expected phase unchanged, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when activate msg arrives in wrong phase")
	}
}

func TestUnifiedSession_RuntimeContextDoneMsg_MatchingSeq(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtimeSeq = 5
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.runtimeSeq = 5
	m.runtime = &rt

	result, cmd := m.Update(runtimeContextDoneMsg{seq: 5})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime after context done, got %d", updated.phase)
	}
	if updated.runtime != nil {
		t.Fatal("expected runtime to be nil after context done")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after context done")
	}
	select {
	case event := <-events:
		if event.kind != unifiedEventRuntimeDisconnected {
			t.Fatalf("expected unifiedEventRuntimeDisconnected, got %d", event.kind)
		}
	default:
		t.Fatal("expected runtime disconnected event")
	}
}

func TestUnifiedSession_RuntimeContextDoneMsg_StaleSeq(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.runtimeSeq = 5
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	m.runtime = &rt

	result, cmd := m.Update(runtimeContextDoneMsg{seq: 3})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseRuntime {
		t.Fatalf("expected phase unchanged for stale seq, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for stale seq")
	}
}

func TestUnifiedSession_ContextDoneMsg_Quits(t *testing.T) {
	m, events := newTestUnifiedModel(t)

	result, cmd := m.Update(contextDoneMsg{})
	_ = result.(unifiedSessionModel)
	if cmd == nil {
		t.Fatal("expected quit cmd on context done")
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

func TestUnifiedSession_WindowSizeMsg_Propagates(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := result.(unifiedSessionModel)
	if updated.width != 120 || updated.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", updated.width, updated.height)
	}
}

func TestUnifiedSession_Configurator_UserExit(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.configurator.done = true
	m.configurator.resultErr = ErrConfiguratorSessionUserExit

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	_ = result.(unifiedSessionModel)
	if cmd == nil {
		t.Fatal("expected quit cmd for user exit")
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

func TestUnifiedSession_Configurator_ModeSelected(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.configurator.done = true
	m.configurator.resultMode = mode.Client

	result, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", updated.phase)
	}
	select {
	case event := <-events:
		if event.kind != unifiedEventModeSelected {
			t.Fatalf("expected unifiedEventModeSelected, got %d", event.kind)
		}
		if event.mode != mode.Client {
			t.Fatalf("expected mode.Client, got %v", event.mode)
		}
	default:
		t.Fatal("expected mode selected event")
	}
}

func TestUnifiedSession_UpdateRuntime_ReconfigureError(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	rt.reconfigureRequested = true
	m.runtime = &rt
	// Make configOpts invalid so newConfiguratorSessionModel fails on reconfigure.
	m.configOpts.ServerConfigManager = nil

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	_ = result.(unifiedSessionModel)
	if cmd == nil {
		t.Fatal("expected quit cmd on reconfigure error")
	}
	select {
	case event := <-events:
		if event.kind != unifiedEventError {
			t.Fatalf("expected unifiedEventError, got %d", event.kind)
		}
		if event.err == nil {
			t.Fatal("expected non-nil error in event")
		}
	default:
		t.Fatal("expected error event")
	}
}

// --- UnifiedSession external handle tests ---

func TestUnifiedSession_WaitForMode_ModeSelected(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventModeSelected, mode: mode.Server}
	}()

	m, err := s.WaitForMode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if m != mode.Server {
		t.Fatalf("expected mode.Server, got %v", m)
	}
}

func TestUnifiedSession_WaitForMode_ExitEvent(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventExit}
	}()

	_, err := s.WaitForMode()
	if !errors.Is(err, ErrUnifiedSessionQuit) {
		t.Fatalf("expected ErrUnifiedSessionQuit, got %v", err)
	}
}

func TestUnifiedSession_WaitForMode_ErrorEvent(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventError, err: errors.New("some error")}
	}()

	_, err := s.WaitForMode()
	if err == nil || err.Error() != "some error" {
		t.Fatalf("expected 'some error', got %v", err)
	}
}

func TestUnifiedSession_WaitForMode_ReconfigureSkippedThenMode(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventReconfigure}
		events <- unifiedEvent{kind: unifiedEventModeSelected, mode: mode.Client}
	}()

	m, err := s.WaitForMode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if m != mode.Client {
		t.Fatalf("expected mode.Client, got %v", m)
	}
}

func TestUnifiedSession_WaitForMode_ChannelClosed(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	close(events)

	_, err := s.WaitForMode()
	if !errors.Is(err, ErrUnifiedSessionClosed) {
		t.Fatalf("expected ErrUnifiedSessionClosed, got %v", err)
	}
}

func TestUnifiedSession_WaitForMode_DoneWithError(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done, err: errors.New("program error")}

	close(done)

	_, err := s.WaitForMode()
	if err == nil || err.Error() != "program error" {
		t.Fatalf("expected 'program error', got %v", err)
	}
}

func TestUnifiedSession_WaitForMode_DoneWithoutError(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	close(done)

	_, err := s.WaitForMode()
	if !errors.Is(err, ErrUnifiedSessionClosed) {
		t.Fatalf("expected ErrUnifiedSessionClosed, got %v", err)
	}
}

func TestUnifiedSession_WaitForRuntimeExit_Reconfigure(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventReconfigure}
	}()

	reconfigure, err := s.WaitForRuntimeExit()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reconfigure {
		t.Fatal("expected reconfigure=true")
	}
}

func TestUnifiedSession_WaitForRuntimeExit_Disconnected(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventRuntimeDisconnected}
	}()

	reconfigure, err := s.WaitForRuntimeExit()
	if !errors.Is(err, ErrUnifiedSessionRuntimeDisconnected) {
		t.Fatalf("expected ErrUnifiedSessionRuntimeDisconnected, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
}

func TestUnifiedSession_WaitForRuntimeExit_ExitEvent(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventExit}
	}()

	_, err := s.WaitForRuntimeExit()
	if !errors.Is(err, ErrUnifiedSessionQuit) {
		t.Fatalf("expected ErrUnifiedSessionQuit, got %v", err)
	}
}

func TestUnifiedSession_WaitForRuntimeExit_ErrorEvent(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventError, err: errors.New("runtime error")}
	}()

	_, err := s.WaitForRuntimeExit()
	if err == nil || err.Error() != "runtime error" {
		t.Fatalf("expected 'runtime error', got %v", err)
	}
}

func TestUnifiedSession_WaitForRuntimeExit_ModeSelected(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	go func() {
		events <- unifiedEvent{kind: unifiedEventModeSelected, mode: mode.Client}
	}()

	reconfigure, err := s.WaitForRuntimeExit()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reconfigure {
		t.Fatal("expected reconfigure=true for unexpected modeSelected")
	}
}

func TestUnifiedSession_WaitForRuntimeExit_ChannelClosed(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	close(events)

	_, err := s.WaitForRuntimeExit()
	if !errors.Is(err, ErrUnifiedSessionClosed) {
		t.Fatalf("expected ErrUnifiedSessionClosed, got %v", err)
	}
}

func TestUnifiedSession_WaitForRuntimeExit_DoneWithError(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done, err: errors.New("program error")}

	close(done)

	_, err := s.WaitForRuntimeExit()
	if err == nil || err.Error() != "program error" {
		t.Fatalf("expected 'program error', got %v", err)
	}
}

func TestUnifiedSession_WaitForRuntimeExit_DoneWithoutError(t *testing.T) {
	events := make(chan unifiedEvent, 4)
	done := make(chan struct{})
	s := &UnifiedSession{events: events, done: done}

	close(done)

	_, err := s.WaitForRuntimeExit()
	if !errors.Is(err, ErrUnifiedSessionClosed) {
		t.Fatalf("expected ErrUnifiedSessionClosed, got %v", err)
	}
}

func TestUnifiedSession_Done_ReturnsChannel(t *testing.T) {
	done := make(chan struct{})
	s := &UnifiedSession{done: done}

	ch := s.Done()
	if ch == nil {
		t.Fatal("expected non-nil done channel")
	}

	// Should not be closed initially.
	select {
	case <-ch:
		t.Fatal("expected done channel to be open")
	default:
	}

	close(done)
	select {
	case <-ch:
	default:
		t.Fatal("expected done channel to be closed after close(done)")
	}
}

func withNoTTYUnifiedSession(t *testing.T) {
	t.Helper()
	prev := newUnifiedSessionProgram
	newUnifiedSessionProgram = func(model tea.Model) *tea.Program {
		return tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	}
	t.Cleanup(func() { newUnifiedSessionProgram = prev })
}

func TestNewUnifiedSession_Success(t *testing.T) {
	withNoTTYUnifiedSession(t)
	ctx, cancel := context.WithCancel(context.Background())

	session, err := NewUnifiedSession(ctx, defaultUnifiedConfigOpts())
	if err != nil {
		t.Fatalf("NewUnifiedSession error: %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.Done() == nil {
		t.Fatal("expected non-nil Done channel")
	}

	// Cancel context to make the session exit.
	cancel()
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not complete after context cancel")
	}
}

func TestNewUnifiedSession_InvalidOpts(t *testing.T) {
	_, err := NewUnifiedSession(context.Background(), ConfiguratorSessionOptions{})
	if err == nil {
		t.Fatal("expected error for empty opts")
	}
}

func TestUnifiedSession_ActivateRuntime_SendsMessage(t *testing.T) {
	withNoTTYUnifiedSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, err := NewUnifiedSession(ctx, defaultUnifiedConfigOpts())
	if err != nil {
		t.Fatalf("NewUnifiedSession error: %v", err)
	}

	// ActivateRuntime should not panic even if session is in configuring phase.
	session.ActivateRuntime(ctx, RuntimeDashboardOptions{Mode: RuntimeDashboardServer})

	cancel()
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not complete")
	}
}

func TestUnifiedSession_Close(t *testing.T) {
	withNoTTYUnifiedSession(t)
	ctx := context.Background()

	session, err := NewUnifiedSession(ctx, defaultUnifiedConfigOpts())
	if err != nil {
		t.Fatalf("NewUnifiedSession error: %v", err)
	}

	// Close should complete without deadlock.
	done := make(chan struct{})
	go func() {
		session.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete")
	}

	// Double close should be safe.
	session.Close()
}

func TestUnifiedSession_RuntimePhase_ShowsRuntimeView(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	m.width = 100
	m.height = 30
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		Mode: RuntimeDashboardServer,
	}, testSettings())
	rt.width = 100
	rt.height = 30
	m.runtime = &rt

	view := m.View().Content
	if !strings.Contains(view, "Mode: Server") {
		t.Fatalf("expected runtime view with server mode, got: %q", view)
	}
}

// --- Phase: fatalError ---

func TestUnifiedSession_FatalErrorMsg_TransitionsToFatalError(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.width = 100
	m.height = 30

	result, cmd := m.Update(fatalErrorMsg{message: "something failed"})
	updated := result.(unifiedSessionModel)

	if updated.phase != phaseFatalError {
		t.Fatalf("expected phaseFatalError, got %d", updated.phase)
	}
	if updated.fatalError == nil {
		t.Fatal("expected fatalError model to be set")
	}
	if updated.fatalError.message != "something failed" {
		t.Fatalf("expected message 'something failed', got %q", updated.fatalError.message)
	}
	if updated.fatalError.width != 100 || updated.fatalError.height != 30 {
		t.Fatalf("expected fatalError dimensions 100x30, got %dx%d", updated.fatalError.width, updated.fatalError.height)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd on fatal error transition")
	}
}

func TestUnifiedSession_FatalErrorMsg_FromRuntimePhase(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseRuntime
	rt := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}, testSettings())
	m.runtime = &rt

	result, _ := m.Update(fatalErrorMsg{message: "port in use"})
	updated := result.(unifiedSessionModel)

	if updated.phase != phaseFatalError {
		t.Fatalf("expected phaseFatalError, got %d", updated.phase)
	}
	if updated.fatalError == nil {
		t.Fatal("expected fatalError model to be set")
	}
}

func TestUnifiedSession_FatalErrorPhase_ViewDelegatesToFatalError(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	fe := newFatalErrorModel("Details here", testSettings())
	fe.width = 100
	fe.height = 30
	m.fatalError = &fe
	m.phase = phaseFatalError

	view := m.View().Content
	if !strings.Contains(view, "Details here") {
		t.Fatalf("expected fatal error message in view, got: %q", view)
	}
}

func TestUnifiedSession_FatalErrorPhase_EnterSendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	fe := newFatalErrorModel("details", testSettings())
	m.fatalError = &fe
	m.phase = phaseFatalError

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit cmd when fatal error dismissed")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on fatal error dismiss")
	}
}

func TestUnifiedSession_FatalErrorPhase_EscSendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	fe := newFatalErrorModel("details", testSettings())
	m.fatalError = &fe
	m.phase = phaseFatalError

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected quit cmd when fatal error dismissed via Esc")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on fatal error Esc dismiss")
	}
}

func TestUnifiedSession_FatalErrorPhase_QKeySendsExitEvent(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	fe := newFatalErrorModel("details", testSettings())
	m.fatalError = &fe
	m.phase = phaseFatalError

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected quit cmd when fatal error dismissed via 'q'")
	}

	select {
	case event := <-events:
		if event.kind != unifiedEventExit {
			t.Fatalf("expected unifiedEventExit, got %d", event.kind)
		}
	default:
		t.Fatal("expected exit event on fatal error 'q' dismiss")
	}
}

func TestUnifiedSession_FatalErrorPhase_ArbitraryKeyNoQuit(t *testing.T) {
	m, events := newTestUnifiedModel(t)
	fe := newFatalErrorModel("details", testSettings())
	m.fatalError = &fe
	m.phase = phaseFatalError

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Fatal("expected nil cmd for arbitrary key in fatal error phase")
	}

	select {
	case event := <-events:
		t.Fatalf("expected no event for arbitrary key, got %d", event.kind)
	default:
	}
}

func TestUnifiedSession_FatalErrorPhase_WindowSizeUpdates(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	fe := newFatalErrorModel("details", testSettings())
	m.fatalError = &fe
	m.phase = phaseFatalError

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := result.(unifiedSessionModel)
	if updated.fatalError.width != 120 || updated.fatalError.height != 40 {
		t.Fatalf("expected fatalError dimensions 120x40, got %dx%d", updated.fatalError.width, updated.fatalError.height)
	}
}

func TestUnifiedSession_FatalErrorPhase_NilFatalError_View(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseFatalError
	m.fatalError = nil

	view := m.View().Content
	if view != "" {
		t.Fatalf("expected empty view when fatalError is nil, got: %q", view)
	}
}

func TestUnifiedSession_FatalErrorPhase_NilFatalError_Update(t *testing.T) {
	m, _ := newTestUnifiedModel(t)
	m.phase = phaseFatalError
	m.fatalError = nil

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := result.(unifiedSessionModel)
	if updated.phase != phaseFatalError {
		t.Fatalf("expected phase unchanged, got %d", updated.phase)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when fatalError is nil")
	}
}

func TestUnifiedSession_ShowFatalError_BlocksUntilDone(t *testing.T) {
	withNoTTYUnifiedSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, err := NewUnifiedSession(ctx, defaultUnifiedConfigOpts())
	if err != nil {
		t.Fatalf("NewUnifiedSession error: %v", err)
	}

	unblocked := make(chan struct{})
	go func() {
		session.ShowFatalError("test details")
		close(unblocked)
	}()

	// Cancel context to make the program exit, which unblocks ShowFatalError.
	cancel()

	select {
	case <-unblocked:
	case <-time.After(5 * time.Second):
		t.Fatal("ShowFatalError did not unblock after context cancel")
	}
}
