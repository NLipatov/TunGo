package bubble_tea

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	tuiconfig "tungo/internal/config/tui"
	"tungo/internal/daemon/systemd"
	"tungo/internal/mode"
)

func TestDaemonMenu_IndependentServiceLifecycles(t *testing.T) {
	opts := defaultConfiguratorOpts()
	daemon := opts.testDaemon()
	states := map[mode.Mode]systemd.UnitStatus{}
	daemon.status = func(role mode.Mode) (systemd.UnitStatus, error) { return states[role], nil }
	install := func(role mode.Mode) (string, error) {
		states[role] = systemd.UnitStatus{Installed: true, Managed: true, ActiveState: "inactive", UnitFileState: "enabled"}
		return "unit", nil
	}
	daemon.setupClient = func() (string, error) { return install(mode.Client) }
	daemon.setupServer = func() (string, error) { return install(mode.Server) }
	var called daemonMenuOption
	daemon.start = func(role mode.Mode) error {
		called = daemonMenuOption{role, daemonStartLabel}
		state := states[role]
		state.ActiveState = "active"
		states[role] = state
		return nil
	}
	daemon.stop = func(role mode.Mode) error {
		called = daemonMenuOption{role, daemonStopLabel}
		state := states[role]
		state.ActiveState = "inactive"
		states[role] = state
		return nil
	}
	daemon.enable = func(role mode.Mode) error {
		called = daemonMenuOption{role, daemonEnableLabel}
		state := states[role]
		state.UnitFileState = "enabled"
		states[role] = state
		return nil
	}
	daemon.disable = func(role mode.Mode) error {
		called = daemonMenuOption{role, daemonDisableLabel}
		state := states[role]
		state.UnitFileState = "disabled"
		states[role] = state
		return nil
	}
	daemon.delete = func(role mode.Mode) error {
		called = daemonMenuOption{role, daemonDeleteLabel}
		delete(states, role)
		return nil
	}
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	model.screen = configuratorScreenDaemonManage
	model = completeDaemonStart(model.refreshDaemonStatus())
	if !slices.Equal(model.daemon.menuOptions, daemonTestOptions(daemonSetupClientLabel, daemonSetupServerLabel)) {
		t.Fatalf("initial actions: %v", model.daemon.menuOptions)
	}
	execute := func(option daemonMenuOption) {
		t.Helper()
		model.cursor = slices.Index(model.daemon.menuOptions, option)
		if model.cursor < 0 {
			t.Fatalf("missing %v in %v", option, model.daemon.menuOptions)
		}
		result, _ := completeDaemonCommands(model.Update(keyNamed(tea.KeyEnter)))
		model = result.(Configurator)
		if model.notice != "" {
			t.Fatalf("unexpected failure: %s", model.notice)
		}
	}
	execute(daemonMenuOption{mode.Client, daemonSetupClientLabel})
	if !slices.Contains(model.daemon.menuOptions, daemonMenuOption{mode.Server, daemonSetupServerLabel}) {
		t.Fatal("installing client hid server setup")
	}
	execute(daemonMenuOption{mode.Server, daemonSetupServerLabel})
	for _, role := range []mode.Mode{mode.Client, mode.Server} {
		execute(daemonMenuOption{role, daemonStartLabel})
		if called != (daemonMenuOption{role, daemonStartLabel}) {
			t.Fatalf("wrong service started: %v", called)
		}
	}
	if states[mode.Client].ActiveState != "active" || states[mode.Server].ActiveState != "active" {
		t.Fatal("both services must remain active")
	}
	for _, role := range []mode.Mode{mode.Client, mode.Server} {
		other := mode.Client
		if role == mode.Client {
			other = mode.Server
		}
		for _, action := range []string{daemonDisableLabel, daemonEnableLabel, daemonStopLabel, daemonDeleteLabel} {
			otherBefore := states[other]
			option := daemonMenuOption{role, action}
			execute(option)
			if called != option {
				t.Fatalf("wrong operation: got %v, want %v", called, option)
			}
			if states[other] != otherBefore {
				t.Fatalf("%v changed other service", option)
			}
		}
		selected := model.daemon.menuOptions[model.cursor]
		if selected.mode != role || !strings.HasPrefix(selected.action, "setup ") {
			t.Fatalf("deletion moved selection onto another service: %v", selected)
		}
	}
}

func TestDaemonGuard_OnlyChecksAndStopsRequestedRole(t *testing.T) {
	for _, target := range []mode.Mode{mode.Client, mode.Server} {
		for _, ownActive := range []bool{false, true} {
			opts := defaultConfiguratorOpts()
			daemon := opts.testDaemon()
			daemon.isActive = func(role mode.Mode) (bool, error) {
				if role != target {
					t.Fatalf("checked %v instead of %v", role, target)
				}
				return ownActive, nil
			}
			stopped := false
			daemon.stop = func(role mode.Mode) error {
				if role != target {
					t.Fatalf("stopped %v instead of %v", role, target)
				}
				stopped = true
				return nil
			}
			model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
			if err != nil {
				t.Fatal(err)
			}
			model = completeDaemonStart(model.startModeWithDaemonGuard(target, configuratorScreenMode, false))
			if ownActive {
				if model.screen != configuratorScreenDaemonActiveConfirm || model.done {
					t.Fatal("same-role daemon was not guarded")
				}
				result, _ := completeDaemonCommands(model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter)))
				model = result.(Configurator)
			}
			if !model.done || model.resultMode != target || stopped != ownActive {
				t.Fatalf("unexpected guard result: done=%v, mode=%v, stopped=%v", model.done, model.resultMode, stopped)
			}
			model.Close()
		}
	}
}

func TestDaemonMenu_StatusErrorDoesNotHideOtherRole(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().status = func(role mode.Mode) (systemd.UnitStatus, error) {
		if role == mode.Client {
			return systemd.UnitStatus{}, errors.New("client status unavailable")
		}
		return systemd.UnitStatus{Installed: true, Managed: true, ActiveState: "active", UnitFileState: "enabled"}, nil
	}
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	model.screen = configuratorScreenDaemonManage
	model = completeDaemonStart(model.refreshDaemonStatus())
	if len(model.daemon.menuOptions) == 0 {
		t.Fatal("server actions disappeared with client status error")
	}
	for _, option := range model.daemon.menuOptions {
		if option.mode != mode.Server {
			t.Fatalf("client action offered without status: %v", option)
		}
	}
	if !strings.Contains(model.View().Content, "client status unavailable") {
		t.Fatal("status error hidden")
	}
}
