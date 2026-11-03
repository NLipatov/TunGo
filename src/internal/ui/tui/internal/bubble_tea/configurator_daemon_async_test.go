package bubble_tea

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	tuiconfig "tungo/internal/config/tui"
	"tungo/internal/daemon/systemd"
	"tungo/internal/mode"
)

func TestDaemonStart_DelayedCommandDoesNotBlockInput(t *testing.T) {
	for _, stage := range []string{"start", "status"} {
		t.Run(stage, func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			daemon := opts.testDaemon()
			model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
			if err != nil {
				t.Fatal(err)
			}
			model.screen = configuratorScreenDaemonManage
			model.daemon.menuOptions = []string{daemonStartLabel, daemonDisableLabel}
			model.width, model.height = 120, 24

			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			wait := func() { close(entered); <-release }
			daemon.start = func() error {
				if stage == "start" {
					wait()
				}
				return nil
			}
			daemon.status = func() (systemd.UnitStatus, error) {
				if stage == "status" {
					wait()
				}
				return systemd.UnitStatus{Installed: true, ActiveState: "active", UnitFileState: "enabled"}, nil
			}

			type updateResult struct {
				model tea.Model
				cmd   tea.Cmd
			}
			updated := make(chan updateResult, 1)
			go func() { m, cmd := model.Update(keyNamed(tea.KeyEnter)); updated <- updateResult{m, cmd} }()
			var cmd tea.Cmd
			select {
			case result := <-updated:
				model, cmd = result.model.(Configurator), result.cmd
			case <-time.After(time.Second):
				unblock()
				<-updated
				t.Fatal("Enter blocked the UI while waiting for the daemon")
			}
			if cmd == nil {
				t.Fatal("expected a background command")
			}
			if !strings.Contains(model.View().Content, "Starting daemon...") {
				t.Fatal("no progress notice while starting daemon")
			}
			completed := make(chan tea.Msg, 1)
			go func() { completed <- cmd() }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("command did not reach delayed operation")
			}

			m, _ := model.Update(keyNamed(tea.KeyDown))
			model = m.(Configurator)
			if model.cursor != 1 {
				t.Fatal("navigation stopped while the command was pending")
			}
			m, duplicate := model.Update(keyNamed(tea.KeyEnter))
			model = m.(Configurator)
			if duplicate != nil {
				t.Fatal("started another daemon operation while busy")
			}
			quitModel, quit := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			if !quitModel.(Configurator).done || quit == nil {
				t.Fatal("Ctrl+C did not quit while the command was pending")
			}
			m, _ = model.Update(keyNamed(tea.KeyEsc))
			model = m.(Configurator)
			if model.screen != configuratorScreenMode {
				t.Fatal("Esc did not leave the daemon screen")
			}
			cursor := model.cursor
			unblock()
			m, _ = model.Update(<-completed)
			model = m.(Configurator)
			if model.screen != configuratorScreenMode || model.cursor != cursor {
				t.Fatal("late result changed navigation after Esc")
			}
			if model.daemon.status.ActiveState != "active" {
				t.Fatal("daemon status was not updated after completion")
			}
		})
	}
}

type cancellableDaemonControl struct {
	*daemonControlStub
	run func(context.Context) error
}

func (d cancellableDaemonControl) StartUnit(ctx context.Context) error { return d.run(ctx) }
func (d cancellableDaemonControl) StopUnit(ctx context.Context) error  { return d.run(ctx) }
func (d cancellableDaemonControl) IsUnitActive(ctx context.Context) (bool, error) {
	return false, d.run(ctx)
}

func TestConfiguratorClose_CancelsAndWaitsForDaemonCommands(t *testing.T) {
	for _, operation := range []string{"start", "check", "stop"} {
		t.Run(operation, func(t *testing.T) {
			started, cancelled, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			opts := defaultConfiguratorOpts()
			opts.Daemon = cancellableDaemonControl{
				daemonControlStub: newDaemonControlStub(),
				run: func(ctx context.Context) error {
					close(started)
					<-ctx.Done()
					close(cancelled)
					// Model cleanup must wait for the operation to finish handling cancellation.
					<-release
					close(finished)
					return ctx.Err()
				},
			}
			model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
			if err != nil {
				t.Fatal(err)
			}
			original := model
			t.Cleanup(func() { unblock(); original.Close() })
			switch operation {
			case "start":
				model.screen = configuratorScreenDaemonManage
				model.daemon.menuOptions = []string{daemonStartLabel}
			case "stop":
				model.screen = configuratorScreenDaemonActiveConfirm
				model.pendingStartMode, model.pendingStartScreen = mode.Server, configuratorScreenServerSelect
			}
			_, cmd := model.Update(keyNamed(tea.KeyEnter))
			if cmd == nil {
				t.Fatal("expected an asynchronous operation")
			}
			completed := make(chan tea.Msg, 1)
			go func() { completed <- cmd() }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("operation did not start")
			}
			closed := make(chan struct{})
			go func() { original.Close(); close(closed) }()
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("Close did not cancel the operation")
			}
			select {
			case <-closed:
				t.Fatal("Close returned before command cleanup finished")
			default:
			}
			unblock()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("Close did not finish after cleanup")
			}
			select {
			case <-finished:
			default:
				t.Fatal("operation still running after Close")
			}
			<-completed
		})
	}
}

func TestConfiguratorClose_PreventsQueuedCommandFromStarting(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().start = func() error { t.Fatal("queued command started after Close"); return nil }
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
	if err != nil {
		t.Fatal(err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{daemonStartLabel}
	_, cmd := model.Update(keyNamed(tea.KeyEnter))
	closed := make(chan struct{})
	go func() { model.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		// Release an incorrectly pre-registered command so the test can fail cleanly.
		cmd()
		<-closed
		t.Fatal("Close waited for a command that had not started")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("queued command executed after Close: %T", msg)
	}
}

func TestDaemonGuard_RefreshAfterCancellationReportsStatusFailure(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return systemd.UnitStatus{}, errors.New("status unavailable") }
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	model.screen = configuratorScreenDaemonManage
	model.daemon.pending = "Checking daemon status..."
	next, cmd := model.Update(daemonActiveMsg{})
	if cmd == nil {
		t.Fatal("expected status refresh")
	}
	next, _ = completeDaemonCommands(next, cmd)
	model = next.(Configurator)
	if model.daemon.pending != "" || !strings.Contains(model.View().Content, "status unavailable") {
		t.Fatal("failed refresh left the menu busy or hid the error")
	}
}

func TestDaemonStart_TimeoutRefreshesStatusAndAllowsNextAction(t *testing.T) {
	opts := defaultConfiguratorOpts()
	daemon := opts.testDaemon()
	daemon.start = func() error { return context.DeadlineExceeded }
	daemon.status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: true, ActiveState: "active", SubState: "running"}, nil
	}
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
	if err != nil {
		t.Fatal(err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{daemonStartLabel}
	m, _ := completeDaemonCommands(model.Update(keyNamed(tea.KeyEnter)))
	model = m.(Configurator)
	if !strings.Contains(model.notice, "timed out") || model.daemon.pending != "" {
		t.Fatalf("timeout did not release the menu with an error: %+v", model.daemon)
	}
	if model.daemon.menuOptions[0] != daemonStopLabel {
		t.Fatal("timeout hid the service's actual running state")
	}
	_, cmd := model.Update(keyNamed(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("next action is still blocked after timeout")
	}
}

func TestDaemonPendingOperation_DoesNotAllowConflictingActionsAfterEsc(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		t.Fatal("unexpected synchronous status query")
		return systemd.UnitStatus{}, nil
	}
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
	if err != nil {
		t.Fatal(err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.pending = "Starting daemon..."
	m, _ := model.Update(keyNamed(tea.KeyEsc))
	model = m.(Configurator)
	m, cmd := model.Update(keyNamed(tea.KeyEnter))
	model = m.(Configurator)
	if cmd != nil || model.screen != configuratorScreenDaemonManage {
		t.Fatal("reentering daemon screen launched a second query")
	}
	m, _ = model.Update(keyNamed(tea.KeyEsc))
	model = m.(Configurator)
	model.cursor = indexOfString(model.modeOptions, modeServerLabel)
	m, cmd = model.Update(keyNamed(tea.KeyEnter))
	model = m.(Configurator)
	if cmd != nil || model.screen != configuratorScreenMode {
		t.Fatal("started another runtime while daemon operation was pending")
	}
}

func TestDaemonGuard_LateResultsDoNotStartCancelledRuntime(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "check", true: "stop"}[stop], func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			daemon := opts.testDaemon()
			daemon.isActive = func() (bool, error) { return false, nil }
			model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
			if err != nil {
				t.Fatal(err)
			}
			var cmd tea.Cmd
			if stop {
				model.screen = configuratorScreenDaemonActiveConfirm
				model.pendingStartMode, model.pendingStartScreen = mode.Server, configuratorScreenServerSelect
				m, c := model.Update(keyNamed(tea.KeyEnter))
				model, cmd = m.(Configurator), c
			} else {
				model, cmd = model.startModeWithDaemonGuard(mode.Server, configuratorScreenServerSelect, false)
			}
			m, _ := model.Update(keyNamed(tea.KeyEsc))
			model = m.(Configurator)
			m, next := model.Update(cmd())
			model = m.(Configurator)
			if model.done || next != nil || model.pendingStartMode != 0 || model.screen != configuratorScreenServerSelect {
				t.Fatal("late daemon result started a cancelled runtime")
			}
			if model.daemon.pending != "" {
				t.Fatal("cancelled request left the UI busy")
			}
		})
	}
}

func TestDaemonGuard_CompletionRefreshesEnteredDaemonMenu(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "check"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			daemon := opts.testDaemon()
			daemon.isActive = func() (bool, error) { return stop, nil }
			daemon.status = func() (systemd.UnitStatus, error) {
				return systemd.UnitStatus{Installed: true, ActiveState: "inactive", UnitFileState: "enabled"}, nil
			}
			m, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceServer))
			if err != nil {
				t.Fatal(err)
			}
			next, cmd := m.Update(keyNamed(tea.KeyEnter))
			m = next.(Configurator)
			if stop {
				next, _ = m.Update(cmd())
				m = next.(Configurator)
				next, cmd = m.Update(keyNamed(tea.KeyEnter))
				m = next.(Configurator)
			}
			// Withdraw runtime startup, go back to mode selection and open daemon management.
			next, _ = m.Update(keyNamed(tea.KeyEsc))
			m = next.(Configurator)
			next, _ = m.Update(keyNamed(tea.KeyEsc))
			m = next.(Configurator)
			m.cursor = indexOfString(m.modeOptions, modeDaemonLabel)
			next, query := m.Update(keyNamed(tea.KeyEnter))
			m = next.(Configurator)
			if query != nil {
				t.Fatal("unexpected second in-flight operation")
			}
			if m.screen != configuratorScreenDaemonManage {
				t.Fatal("did not open daemon menu")
			}
			next, query = m.Update(cmd())
			m = next.(Configurator)
			if query == nil {
				t.Fatal("completion did not refresh the newly opened daemon menu")
			}
			next, _ = completeDaemonCommands(m, query)
			m = next.(Configurator)
			if !m.daemon.status.Installed || len(m.daemon.menuOptions) == 0 || m.daemon.menuOptions[0] != daemonStartLabel {
				t.Fatalf("daemon page not refreshed after %s completion: pending=%q status=%+v menu=%v", name, m.daemon.pending, m.daemon.status, m.daemon.menuOptions)
			}
		})
	}
}
