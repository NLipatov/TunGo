package bubble_tea

import (
	"errors"
	"strings"
	"testing"

	"tungo/application"
	"tungo/infrastructure/PAL/service_management/linux/systemd"

	tea "charm.land/bubbletea/v2"
)

func TestModeOptions_AddsDaemonWhenDaemonAvailable(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{
			Installed:     true,
			UnitFileState: "enabled",
			ActiveState:   "inactive",
			Role:          systemd.UnitRoleClient,
		}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsString(model.modeOptions, modeDaemonLabel) {
		t.Fatalf("expected daemon option in mode screen, got %v", model.modeOptions)
	}
}

func TestModeOptions_DoesNotAddDaemonWhenDaemonUnavailable(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = nil

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsString(model.modeOptions, modeDaemonLabel) {
		t.Fatalf("expected no daemon option when unsupported, got %v", model.modeOptions)
	}
}

func TestUpdateModeScreen_EnterOnDaemon_OpensDaemonManageScreen(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenMode
	model.cursor = indexOfString(model.modeOptions, modeDaemonLabel)
	if model.cursor < 0 {
		t.Fatalf("expected daemon option in mode options, got %v", model.modeOptions)
	}

	updatedModel, _ := model.updateModeScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
}

func TestUpdateClientSelectScreen_Esc_ServerUnsupportedWithDaemon_ReturnsToModeScreen(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurationControl = nil
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen when daemon option exists, got %v", model.screen)
	}

	model.screen = configuratorScreenClientSelect
	updatedModel, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if updated.done {
		t.Fatal("expected session to stay open")
	}
	if updated.screen != configuratorScreenMode {
		t.Fatalf("expected return to mode screen, got %v", updated.screen)
	}
}

func TestView_ClientSelectHint_ServerUnsupportedWithDaemon_ShowsEscBack(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurationControl = nil
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect

	view := model.View().Content
	if !strings.Contains(view, "Esc back") {
		t.Fatalf("expected 'Esc back' in hint when daemon option exists, got: %s", view)
	}
	if strings.Contains(view, "Esc exit") {
		t.Fatalf("expected no 'Esc exit' in hint when daemon option exists, got: %s", view)
	}
}

func TestDaemonNotice_ShowsNonErrorNotice(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Role: systemd.UnitRoleServer}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.notice = "Reconfigure cancelled."
	notice := model.daemonNotice()
	if !strings.Contains(notice, "Reconfigure cancelled.") {
		t.Fatalf("expected daemon notice to include non-error message, got %q", notice)
	}
}

func TestMainTabView_DaemonManage_SeparatesStatusAndActions(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{
			Installed:      true,
			LoadState:      "loaded",
			UnitFileState:  "enabled",
			ActiveState:    "inactive",
			SubState:       "dead",
			Result:         "success",
			ExecMainStatus: "0",
			ExecStart:      "/usr/local/bin/tungo s",
			Role:           systemd.UnitRoleServer,
		}, nil
	}
	opts.testDaemon().start = func() error { return nil }
	opts.testDaemon().disable = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.notice = "test notice"
	view := model.mainTabView()

	if !strings.Contains(view, "Daemon Status") {
		t.Fatalf("expected status section title, got: %s", view)
	}
	if !strings.Contains(view, "Actions") {
		t.Fatalf("expected actions section title, got: %s", view)
	}
	if !strings.Contains(view, "Updated: ") {
		t.Fatalf("expected updated timestamp in daemon status section, got: %s", view)
	}
	if !strings.Contains(view, "ExecStart: /usr/local/bin/tungo s") {
		t.Fatalf("expected raw ExecStart in status section, got: %s", view)
	}
	if !strings.Contains(view, "DerivedRole: server (from ExecStart)") {
		t.Fatalf("expected derived role from ExecStart in status section, got: %s", view)
	}
	if !strings.Contains(view, "test notice") {
		t.Fatalf("expected daemon notice in body, got: %s", view)
	}
	if !strings.Contains(view, "Tab switch tabs") {
		t.Fatalf("expected daemon manage hint to include tab navigation, got: %s", view)
	}
}

func TestMainTabView_DaemonManage_DerivedRoleFallsBackToMode(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{
			Installed:      true,
			LoadState:      "loaded",
			UnitFileState:  "enabled",
			ActiveState:    "inactive",
			SubState:       "dead",
			Result:         "success",
			ExecMainStatus: "0",
			ExecStart:      "unknown",
			Role:           systemd.UnitRoleServer,
		}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage

	view := model.mainTabView()
	if !strings.Contains(view, "DerivedRole: server (from Role)") {
		t.Fatalf("expected derived role fallback from mode, got: %s", view)
	}
}

func TestUpdateDaemonManageScreen_NotInstalled_ShowsSetupOptions(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}
	opts.testDaemon().setupClient = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.testDaemon().setupServer = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	if !containsString(model.daemon.menuOptions, daemonSetupClientLabel) {
		t.Fatalf("expected setup client option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, daemonSetupServerLabel) {
		t.Fatalf("expected setup server option, got %v", model.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_Installed_ShowsReconfigureOptions(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{
			Installed:     true,
			Managed:       true,
			UnitFileState: "disabled",
			ActiveState:   "inactive",
			Role:          systemd.UnitRoleClient,
		}, nil
	}
	opts.testDaemon().setupClient = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.testDaemon().setupServer = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.testDaemon().delete = func() error { return nil }
	opts.testDaemon().start = func() error { return nil }
	opts.testDaemon().enable = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	if containsString(model.daemon.menuOptions, daemonSetupClientLabel) {
		t.Fatalf("did not expect setup client option for installed daemon, got %v", model.daemon.menuOptions)
	}
	if containsString(model.daemon.menuOptions, daemonSetupServerLabel) {
		t.Fatalf("did not expect setup server option for installed daemon, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, daemonReconfigureClientLabel) {
		t.Fatalf("expected reconfigure client option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, daemonReconfigureServerLabel) {
		t.Fatalf("expected reconfigure server option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, daemonDeleteLabel) {
		t.Fatalf("expected delete daemon option, got %v", model.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_SetupClient_InstallsUnit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	installCalls := 0
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}
	opts.testDaemon().setupClient = func() (string, error) {
		installCalls++
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{daemonSetupClientLabel}
	model.cursor = 0
	model.notice = "stale error"

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if installCalls != 1 {
		t.Fatalf("expected one install call, got %d", installCalls)
	}
	if updated.notice != "" {
		t.Fatalf("expected stale notice to be cleared after successful setup, got %q", updated.notice)
	}
	if strings.Contains(updated.notice, "daemon configured") || strings.Contains(updated.notice, "daemon reconfigured") {
		t.Fatalf("expected no setup/reconfigure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_SetupClient_FailsWhenDefaultConfigInvalid(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: false}, nil
	}
	opts.testDaemon().setupClient = func() (string, error) {
		return "", errors.New("cannot setup client daemon: invalid default config")
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{daemonSetupClientLabel}
	model.cursor = 0

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if !strings.Contains(updated.notice, "cannot setup client daemon") {
		t.Fatalf("expected validation notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureInactive_AppliesImmediately(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Role: systemd.UnitRoleClient}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	reconfigureCalls := 0
	opts.testDaemon().setupServer = func() (string, error) {
		reconfigureCalls++
		status.Role = systemd.UnitRoleServer
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.testDaemon().start = func() error { return nil }
	opts.testDaemon().enable = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, daemonReconfigureServerLabel)
	model.notice = "previous failure"
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", daemonReconfigureServerLabel, model.daemon.menuOptions)
	}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if reconfigureCalls != 1 {
		t.Fatalf("expected one reconfigure call, got %d", reconfigureCalls)
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected to stay on daemon manage screen, got %v", updated.screen)
	}
	if updated.notice != "" {
		t.Fatalf("expected stale notice to be cleared after successful reconfigure, got %q", updated.notice)
	}
	if strings.Contains(updated.notice, "daemon configured") || strings.Contains(updated.notice, "daemon reconfigured") {
		t.Fatalf("expected no setup/reconfigure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureActive_ShowsMandatoryConfirm(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Role: systemd.UnitRoleServer}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	opts.testDaemon().setupClient = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().stop = func() error { return nil }
	opts.testDaemon().start = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, daemonReconfigureClientLabel)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", daemonReconfigureClientLabel, model.daemon.menuOptions)
	}

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd while waiting confirmation")
	}
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected reconfigure confirm screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != application.ModeClient {
		t.Fatalf("expected pending daemon mode client, got %v", updated.pendingDaemonMode)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_Confirm_RestartsWithNewSetup(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Role: systemd.UnitRoleServer}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }

	callOrder := make([]string, 0, 1)
	opts.testDaemon().setupClient = func() (string, error) {
		callOrder = append(callOrder, "setup-client")
		status.Role = systemd.UnitRoleClient
		status.ActiveState = "active"
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = application.ModeClient
	model.cursor = 0 // stop and restart

	updatedModel, cmd := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd on reconfigure")
	}
	if strings.Join(callOrder, ",") != "setup-client" {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	if updated.pendingDaemonMode != 0 {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Client daemon reconfigured") || !strings.Contains(updated.notice, "restarted") {
		t.Fatalf("expected restarted notice, got %q", updated.notice)
	}
	if updated.daemon.status.ActiveState != "active" || updated.daemon.status.Role != systemd.UnitRoleClient {
		t.Fatalf("expected refreshed daemon status (active client), got %+v", updated.daemon.status)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_Cancel_ReturnsToDaemonManage(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = application.ModeServer
	model.cursor = 1 // cancel

	updatedModel, cmd := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd on cancel")
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != 0 {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if !strings.Contains(updated.notice, "Reconfigure cancelled") {
		t.Fatalf("expected cancellation notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_StartEnableDisableStopFlow(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Role: systemd.UnitRoleClient}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	opts.testDaemon().start = func() error {
		status.ActiveState = "active"
		return nil
	}
	opts.testDaemon().stop = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.testDaemon().enable = func() error {
		status.UnitFileState = "enabled"
		return nil
	}
	opts.testDaemon().disable = func() error {
		status.UnitFileState = "disabled"
		return nil
	}
	opts.testDaemon().stop = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.testDaemon().delete = func() error {
		status = systemd.UnitStatus{}
		return nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	model.daemon.menuOptions = []string{daemonStartLabel}
	model.cursor = 0
	next, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(Configurator)
	if status.ActiveState != "active" {
		t.Fatal("expected daemon to be active after start")
	}
	if indexOfString(model.daemon.menuOptions, daemonStopLabel) < 0 {
		t.Fatalf("expected %q option after start, got %v", daemonStopLabel, model.daemon.menuOptions)
	}
	if indexOfString(model.daemon.menuOptions, daemonStartLabel) >= 0 {
		t.Fatalf("did not expect %q option after start, got %v", daemonStartLabel, model.daemon.menuOptions)
	}

	model.daemon.menuOptions = []string{daemonEnableLabel}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(Configurator)
	if status.UnitFileState != "enabled" {
		t.Fatal("expected daemon to be enabled")
	}

	model.daemon.menuOptions = []string{daemonDisableLabel}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(Configurator)
	if status.UnitFileState == "enabled" {
		t.Fatal("expected daemon to be disabled")
	}

	model.daemon.menuOptions = []string{daemonStopLabel}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(Configurator)
	if status.ActiveState == "active" {
		t.Fatal("expected daemon to be stopped")
	}
	if indexOfString(model.daemon.menuOptions, daemonStartLabel) < 0 {
		t.Fatalf("expected %q option after stop, got %v", daemonStartLabel, model.daemon.menuOptions)
	}
	if indexOfString(model.daemon.menuOptions, daemonStopLabel) >= 0 {
		t.Fatalf("did not expect %q option after stop, got %v", daemonStopLabel, model.daemon.menuOptions)
	}
	if model.notice != "" {
		t.Fatalf("expected no success notice after stop, got %q", model.notice)
	}
}

func TestUpdateDaemonManageScreen_StartPreservesActionCursorAfterRefresh(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Role: systemd.UnitRoleClient}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	opts.testDaemon().start = func() error {
		status.ActiveState = "active"
		return nil
	}
	opts.testDaemon().stop = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.testDaemon().disable = func() error { return nil }
	opts.testDaemon().setupClient = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().delete = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{"dummy-before", daemonStartLabel, "dummy-after"}
	model.cursor = 1

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if len(updated.daemon.menuOptions) == 0 {
		t.Fatalf("expected daemon options after refresh")
	}
	if updated.cursor != 1 {
		t.Fatalf("expected cursor to stay on same action slot, got %d; options=%v", updated.cursor, updated.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_Delete_RemovesUnitAndRefreshesStatus(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, Managed: true, UnitFileState: "enabled", ActiveState: "inactive", Role: systemd.UnitRoleServer}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	removeCalls := 0
	opts.testDaemon().delete = func() error {
		removeCalls++
		status = systemd.UnitStatus{}
		return nil
	}
	opts.testDaemon().setupClient = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().setupServer = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, daemonDeleteLabel)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", daemonDeleteLabel, model.daemon.menuOptions)
	}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if removeCalls != 1 {
		t.Fatalf("expected one remove call, got %d", removeCalls)
	}
	if updated.notice != "" {
		t.Fatalf("expected no success notice, got %q", updated.notice)
	}
	if updated.daemon.status.Installed {
		t.Fatalf("expected daemon to be removed, got %+v", updated.daemon.status)
	}
	if !containsString(updated.daemon.menuOptions, daemonSetupClientLabel) {
		t.Fatalf("expected setup options after delete, got %v", updated.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_UnmanagedUnit_HidesDeleteOption(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{
			Installed:     true,
			Managed:       false,
			UnitFileState: "enabled",
			ActiveState:   "inactive",
			Role:          systemd.UnitRoleServer,
		}, nil
	}
	opts.testDaemon().delete = func() error { return nil }
	opts.testDaemon().setupClient = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().setupServer = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	if containsString(model.daemon.menuOptions, daemonDeleteLabel) {
		t.Fatalf("did not expect delete option for unmanaged unit, got %v", model.daemon.menuOptions)
	}
}

func TestUpdateClientSelectScreen_SelectConfig_ActiveDaemon_ShowsStopPrompt(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg-a"}
	opts.testDaemon().isActive = func() (bool, error) { return true, nil }
	opts.testDaemon().stop = func() error { return nil }

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = 0 // cfg-a

	updatedModel, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd when daemon stop confirmation is required")
	}
	if updated.done {
		t.Fatal("expected configurator to stay open for daemon stop confirmation")
	}
	if updated.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected systemd confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != application.ModeClient {
		t.Fatalf("expected pending start mode client, got %v", updated.pendingStartMode)
	}
	if updated.pendingStartScreen != configuratorScreenClientSelect {
		t.Fatalf("expected pending start screen client select, got %v", updated.pendingStartScreen)
	}
	if updated.pendingClientConfig != "cfg-a" {
		t.Fatalf("expected pending client config cfg-a, got %q", updated.pendingClientConfig)
	}
	if s.Current().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged before confirmation, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateServerSelectScreen_Start_ActiveDaemon_ShowsStopPrompt(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().isActive = func() (bool, error) { return true, nil }
	opts.testDaemon().stop = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = indexOfString(model.server.menuOptions, serverStartLabel)

	updatedModel, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd when daemon stop confirmation is required")
	}
	if updated.done {
		t.Fatal("expected configurator to stay open for daemon stop confirmation")
	}
	if updated.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected systemd confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != application.ModeServer {
		t.Fatalf("expected pending start mode server, got %v", updated.pendingStartMode)
	}
}

func TestUpdateDaemonActiveConfirmScreen_EnterStop_StopsDaemonAndStartsMode(t *testing.T) {
	stopCalls := 0
	opts := defaultConfiguratorOpts()
	opts.testDaemon().stop = func() error {
		stopCalls++
		return nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeServer
	model.pendingStartScreen = configuratorScreenServerSelect
	model.cursor = 0 // stop and continue

	updatedModel, cmd := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if stopCalls != 1 {
		t.Fatalf("expected one stop call, got %d", stopCalls)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
	if !updated.done {
		t.Fatal("expected done=true after stop and continue")
	}
	if updated.resultMode != application.ModeServer {
		t.Fatalf("expected application.ModeServer, got %v", updated.resultMode)
	}
}

func TestUpdateDaemonActiveConfirmScreen_Cancel_ReturnsToPreviousScreen(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Current()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "new-cfg"
	model.cursor = 1 // cancel

	updatedModel, cmd := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd on cancel")
	}
	if updated.done {
		t.Fatal("expected done=false on cancel")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select, got %v", updated.screen)
	}
	if updated.pendingStartMode != 0 {
		t.Fatalf("expected pending mode cleared, got %v", updated.pendingStartMode)
	}
	if updated.pendingClientConfig != "" {
		t.Fatalf("expected pending client config cleared, got %q", updated.pendingClientConfig)
	}
	if !strings.Contains(updated.notice, "cancelled") {
		t.Fatalf("expected cancellation notice, got %q", updated.notice)
	}
	if s.Current().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected AutoSelectClientConfig unchanged on cancel, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateDaemonCheckErrorConfirmScreen_RetryCheck_StartsWhenInactive(t *testing.T) {
	checkCalls := 0
	opts := defaultConfiguratorOpts()
	opts.testDaemon().isActive = func() (bool, error) {
		checkCalls++
		if checkCalls == 1 {
			return false, errors.New("probe failed")
		}
		return false, nil
	}
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model = model.startModeWithDaemonGuard(application.ModeServer, configuratorScreenServerSelect, false)
	if model.screen != configuratorScreenDaemonCheckErrorConfirm {
		t.Fatalf("expected check error confirm screen, got %v", model.screen)
	}
	model.cursor = 0 // Retry check

	updatedModel, cmd := model.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd == nil {
		t.Fatal("expected quit cmd after successful retry")
	}
	if !updated.done || updated.resultMode != application.ModeServer {
		t.Fatalf("expected done server start after retry, got done=%v mode=%v", updated.done, updated.resultMode)
	}
	if checkCalls != 2 {
		t.Fatalf("expected 2 check calls, got %d", checkCalls)
	}
}

func TestUpdateDaemonCheckErrorConfirmScreen_RetryCheck_PreservesClientConfig(t *testing.T) {
	checkCalls := 0
	opts := defaultConfiguratorOpts()
	opts.testDaemon().stop = func() error { return nil }
	opts.testDaemon().isActive = func() (bool, error) {
		checkCalls++
		if checkCalls == 1 {
			return false, errors.New("probe failed")
		}
		return true, nil
	}
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("NewConfigurator() error = %v", err)
	}

	model = model.startModeWithDaemonGuard(application.ModeClient, configuratorScreenClientSelect, false)
	model.pendingClientConfig = "cfg-a"
	model.cursor = 0

	updatedModel, cmd := model.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected confirmation screen without command")
	}
	if updated.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("screen = %v, want active confirmation", updated.screen)
	}
	if updated.pendingClientConfig != "cfg-a" {
		t.Fatalf("pending client config = %q, want cfg-a", updated.pendingClientConfig)
	}
}

func TestUpdateDaemonCheckErrorConfirmScreen_StartAnyway_Client_PersistsAutoSelectConfig(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonCheckErrorConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "cfg-a"
	model.cursor = 1 // Start anyway (unsafe)

	updatedModel, cmd := model.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd == nil {
		t.Fatal("expected quit cmd for start anyway")
	}
	if !updated.done || updated.resultMode != application.ModeClient {
		t.Fatalf("expected done client start, got done=%v mode=%v", updated.done, updated.resultMode)
	}
	if !strings.Contains(updated.notice, "without daemon guard") {
		t.Fatalf("expected unsafe start notice, got %q", updated.notice)
	}
	if s.Current().AutoSelectClientConfig != "cfg-a" {
		t.Fatalf("expected AutoSelectClientConfig persisted, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateDaemonCheckErrorConfirmScreen_Cancel_ReturnsToPreviousScreen(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonCheckErrorConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.cursor = 2 // Cancel

	updatedModel, cmd := model.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd on cancel")
	}
	if updated.done {
		t.Fatal("expected done=false on cancel")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select, got %v", updated.screen)
	}
	if updated.pendingStartMode != 0 {
		t.Fatalf("expected pending mode cleared, got %v", updated.pendingStartMode)
	}
}

func TestUpdateDaemonActiveConfirmScreen_StopFails_ShowsNoticeAndReturns(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Current()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testDaemon().stop = func() error { return errors.New("stop failed") }
	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "new-cfg"
	model.cursor = 0 // stop and continue

	updatedModel, cmd := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd when stop fails")
	}
	if updated.done {
		t.Fatal("expected done=false when stop fails")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Failed to stop daemon") {
		t.Fatalf("expected stop failure notice, got %q", updated.notice)
	}
	if s.Current().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected AutoSelectClientConfig unchanged on stop failure, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateDaemonActiveConfirmScreen_EnterStop_Client_PersistsAutoSelectConfig(t *testing.T) {
	stopCalls := 0
	s := settingsForMode(ModePreferenceClient)
	opts := defaultConfiguratorOpts()
	opts.testDaemon().stop = func() error {
		stopCalls++
		return nil
	}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "cfg-a"
	model.cursor = 0

	updatedModel, cmd := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if stopCalls != 1 {
		t.Fatalf("expected one stop call, got %d", stopCalls)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
	if !updated.done {
		t.Fatal("expected done=true after stop and continue")
	}
	if updated.resultMode != application.ModeClient {
		t.Fatalf("expected application.ModeClient, got %v", updated.resultMode)
	}
	if s.Current().AutoSelectClientConfig != "cfg-a" {
		t.Fatalf("expected AutoSelectClientConfig persisted after confirmation, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateDaemonManageScreen_Esc_LeavesDaemonManageScreen(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Role: systemd.UnitRoleServer}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	opts.testDaemon().setupClient = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().setupServer = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().delete = func() error { return nil }
	opts.testDaemon().start = func() error { return nil }
	opts.testDaemon().enable = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.tab = configuratorTabLogs
	model.pendingDaemonMode = application.ModeServer

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(Configurator)
	if updated.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen, got %v", updated.screen)
	}
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected main tab after leave, got %v", updated.tab)
	}
	if updated.pendingDaemonMode != 0 {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if updated.cursor != indexOfString(updated.modeOptions, modeDaemonLabel) {
		t.Fatalf("expected cursor on daemon mode option, got %d (options=%v)", updated.cursor, updated.modeOptions)
	}
}

func TestRefreshDaemonStatus_UnavailableAndError(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.daemon.status = systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Role: systemd.UnitRoleServer}
	model.daemon.menuOptions = []string{daemonStopLabel}

	model.refreshDaemonStatus()
	if model.daemon.statusErr == nil || !strings.Contains(model.daemon.statusErr.Error(), "unavailable") {
		t.Fatalf("expected unavailable status error, got %v", model.daemon.statusErr)
	}
	if model.daemon.status.Installed || len(model.daemon.menuOptions) != 0 {
		t.Fatalf("expected daemon status/menu to be reset, got status=%+v menu=%v", model.daemon.status, model.daemon.menuOptions)
	}

	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return systemd.UnitStatus{}, errors.New("status boom") }
	model, err = NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.refreshDaemonStatus()
	if model.daemon.statusErr == nil || model.daemon.statusErr.Error() != "status boom" {
		t.Fatalf("expected status boom, got %v", model.daemon.statusErr)
	}
}

func TestDaemonStatusLineAndNotice_ErrorAndEmptyNotice(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.daemon.statusErr = errors.New("probe failed")
	if got := model.daemonStatusLine(); !strings.Contains(got, "Status error: probe failed") {
		t.Fatalf("expected status error line, got %q", got)
	}

	model.daemon.statusErr = nil
	model.daemon.status = systemd.UnitStatus{Installed: true, UnitFileState: "disabled", ActiveState: "active", Role: systemd.UnitRoleClient}
	model.notice = ""
	want := model.daemonStatusLine()
	if got := model.daemonNotice(); got != want {
		t.Fatalf("expected daemonNotice to return status line when notice empty, got %q want %q", got, want)
	}
}

func TestUpdateDaemonManageScreen_ActionFailures_ShowNotice(t *testing.T) {
	cases := []struct {
		name           string
		option         string
		active         bool
		configureHooks func(*ConfiguratorOptions)
		wantMsg        string
	}{
		{
			name:   "setup client install fails",
			option: daemonSetupClientLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().setupClient = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup daemon",
		},
		{
			name:   "setup server install fails",
			option: daemonSetupServerLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().setupServer = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup daemon",
		},
		{
			name:   "reconfigure inactive client install fails",
			option: daemonReconfigureClientLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().setupClient = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup daemon",
		},
		{
			name:   "reconfigure inactive server install fails",
			option: daemonReconfigureServerLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().setupServer = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup daemon",
		},
		{
			name:   "start fails",
			option: daemonStartLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().start = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to start daemon: boom",
		},
		{
			name:   "stop fails",
			option: daemonStopLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().stop = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to stop daemon: boom",
		},
		{
			name:   "enable fails",
			option: daemonEnableLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().enable = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to enable daemon: boom",
		},
		{
			name:   "disable fails",
			option: daemonDisableLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().disable = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to disable daemon: boom",
		},
		{
			name:   "delete fails",
			option: daemonDeleteLabel,
			configureHooks: func(opts *ConfiguratorOptions) {
				opts.testDaemon().delete = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to remove daemon: boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			opts.Daemon = newDaemonControlStub()
			opts.testDaemon().status = func() (systemd.UnitStatus, error) {
				return systemd.UnitStatus{Installed: true, ActiveState: boolToActiveState(tc.active), Role: systemd.UnitRoleClient}, nil
			}
			tc.configureHooks(&opts)

			model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			model.screen = configuratorScreenDaemonManage
			model.daemon.status.ActiveState = boolToActiveState(tc.active)
			model.daemon.menuOptions = []string{tc.option}
			model.cursor = 0

			updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
			updated := updatedModel.(Configurator)
			if !strings.Contains(updated.notice, tc.wantMsg) {
				t.Fatalf("expected %q in notice, got %q", tc.wantMsg, updated.notice)
			}
		})
	}
}

func TestUpdateDaemonManageScreen_UnknownOption_Noop(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: true}, nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.notice = "keep"
	model.daemon.menuOptions = []string{"unknown-action"}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if updated.notice != "keep" {
		t.Fatalf("expected notice to stay unchanged, got %q", updated.notice)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_EscAndNonEnter(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = application.ModeServer
	model.cursor = 0

	updatedModel, _ := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(Configurator)
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected to stay on confirm screen on non-enter, got %v", updated.screen)
	}

	updatedModel, _ = updated.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEsc))
	updated = updatedModel.(Configurator)
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen on esc, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != 0 {
		t.Fatalf("expected pending mode cleared on esc, got %v", updated.pendingDaemonMode)
	}
	if !strings.Contains(updated.notice, "Reconfigure cancelled.") {
		t.Fatalf("expected cancel notice on esc, got %q", updated.notice)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_ConfirmServerError_ShowsNotice(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: true, ActiveState: "active", Role: systemd.UnitRoleClient}, nil
	}
	opts.testDaemon().setupServer = func() (string, error) { return "", errors.New("setup failed") }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = application.ModeServer
	model.cursor = 0

	updatedModel, _ := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if !strings.Contains(updated.notice, "setup failed") {
		t.Fatalf("expected setup failure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonActiveConfirmScreen_EscAndStopUnavailable(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "cfg-a"

	updatedModel, _ := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(Configurator)
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select on esc, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Start cancelled.") {
		t.Fatalf("expected cancel notice on esc, got %q", updated.notice)
	}

	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeServer
	model.pendingStartScreen = configuratorScreenServerSelect
	model.cursor = 0
	updatedModel, _ = model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated = updatedModel.(Configurator)
	if updated.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select when stop unavailable, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Stopping daemon is unavailable.") {
		t.Fatalf("expected unavailable notice, got %q", updated.notice)
	}
}

func TestStartModeWithDaemonGuard_PreserveNotice(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().isActive = func() (bool, error) { return true, nil }
	opts.testDaemon().stop = func() error { return nil }
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.notice = "keep me"

	updated := model.startModeWithDaemonGuard(application.ModeServer, configuratorScreenServerSelect, true)
	if updated.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected confirm screen, got %v", updated.screen)
	}
	if updated.notice != "keep me" {
		t.Fatalf("expected notice to be preserved, got %q", updated.notice)
	}
	if updated.pendingStartMode != application.ModeServer || updated.pendingStartScreen != configuratorScreenServerSelect {
		t.Fatalf("expected pending start to be set, got mode=%v screen=%v", updated.pendingStartMode, updated.pendingStartScreen)
	}
}

func TestPersistAutoSelectClientConfig_EmptyValueIgnored(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Current()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	model, err := NewConfigurator(defaultConfiguratorOpts(), s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = model.persistAutoSelectClientConfig("   ")
	if s.Current().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected old config to remain unchanged, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestStartModeWithDaemonGuard_CoversBranches(t *testing.T) {
	t.Run("without hooks starts immediately", func(t *testing.T) {
		model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithDaemonGuard(application.ModeServer, configuratorScreenServerSelect, false)
		if !updated.done || updated.resultMode != application.ModeServer {
			t.Fatalf("expected immediate start, got done=%v mode=%v", updated.done, updated.resultMode)
		}
	})

	t.Run("status check error", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon().isActive = func() (bool, error) { return false, errors.New("status failed") }
		opts.testDaemon().stop = func() error { return nil }
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithDaemonGuard(application.ModeServer, configuratorScreenServerSelect, false)
		if updated.screen != configuratorScreenDaemonCheckErrorConfirm {
			t.Fatalf("expected daemon check error confirm screen, got %v", updated.screen)
		}
		if !strings.Contains(updated.notice, "Failed to check daemon status") {
			t.Fatalf("expected status failure notice, got %q", updated.notice)
		}
		if updated.pendingStartMode != application.ModeServer || updated.pendingStartScreen != configuratorScreenServerSelect {
			t.Fatalf("expected pending start to be set, got mode=%v screen=%v", updated.pendingStartMode, updated.pendingStartScreen)
		}
	})

	t.Run("inactive daemon starts immediately", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon().isActive = func() (bool, error) { return false, nil }
		opts.testDaemon().stop = func() error { return nil }
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithDaemonGuard(application.ModeClient, configuratorScreenClientSelect, false)
		if !updated.done || updated.resultMode != application.ModeClient {
			t.Fatalf("expected immediate client start, got done=%v mode=%v", updated.done, updated.resultMode)
		}
	})

	t.Run("active daemon clears notice when not preserving", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon().isActive = func() (bool, error) { return true, nil }
		opts.testDaemon().stop = func() error { return nil }
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		model.notice = "temporary notice"
		updated := model.startModeWithDaemonGuard(application.ModeServer, configuratorScreenServerSelect, false)
		if updated.screen != configuratorScreenDaemonActiveConfirm {
			t.Fatalf("expected confirm screen, got %v", updated.screen)
		}
		if updated.notice != "" {
			t.Fatalf("expected notice to be cleared, got %q", updated.notice)
		}
	})
}

func TestLeaveDaemonManageScreen_WithoutDaemonModeOption_ResetsCursor(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.modeOptions = []string{modeClientLabel, modeServerLabel}
	model.cursor = 1

	updated := model.leaveDaemonManageScreen()
	if updated.cursor != 0 {
		t.Fatalf("expected cursor reset to 0 when daemon option missing, got %d", updated.cursor)
	}
}

func TestUpdateDaemonManageScreen_NonEnter_DoesNothing(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) {
		return systemd.UnitStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Role: systemd.UnitRoleClient}, nil
	}
	startCalls := 0
	opts.testDaemon().start = func() error {
		startCalls++
		return nil
	}

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{daemonStartLabel}
	model.cursor = 0

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatalf("expected nil cmd on non-enter, got %v", cmd)
	}
	if startCalls != 0 {
		t.Fatalf("expected no start calls on non-enter, got %d", startCalls)
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected to stay on daemon manage screen, got %v", updated.screen)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureServerActive_ShowsMandatoryConfirm(t *testing.T) {
	status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Role: systemd.UnitRoleClient}
	opts := defaultConfiguratorOpts()
	opts.Daemon = newDaemonControlStub()
	opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
	opts.testDaemon().setupServer = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.testDaemon().stop = func() error { return nil }
	opts.testDaemon().start = func() error { return nil }

	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, daemonReconfigureServerLabel)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", daemonReconfigureServerLabel, model.daemon.menuOptions)
	}

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected nil cmd while waiting confirmation")
	}
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected reconfigure confirm screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != application.ModeServer {
		t.Fatalf("expected pending daemon mode server, got %v", updated.pendingDaemonMode)
	}
}

func TestUpdateDaemonActiveConfirmScreen_NonEnter_DoesNothing(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect
	model.cursor = 0

	updatedModel, cmd := model.updateDaemonActiveConfirmScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatalf("expected nil cmd on non-enter, got %v", cmd)
	}
	if updated.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected to stay on confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != application.ModeClient {
		t.Fatalf("expected pending mode to remain unchanged, got %v", updated.pendingStartMode)
	}
}

func TestApplyDaemonSetup_RestartBranchesAndUnknownMode(t *testing.T) {
	t.Run("unavailable daemon returns explicit error", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		model.options.Daemon = nil

		_, err = model.applyDaemonSetup(application.ModeClient, false)
		if err == nil || !strings.Contains(err.Error(), "daemon setup is unavailable") {
			t.Fatalf("expected unavailable daemon error, got %v", err)
		}
	})

	t.Run("invalid client configuration prevents setup", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon()
		opts.testControl().validateActiveErr = errors.New("invalid client config")
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = model.applyDaemonSetup(application.ModeClient, false)
		if err == nil || !strings.Contains(err.Error(), "cannot setup client daemon: invalid client config") {
			t.Fatalf("expected client validation error, got %v", err)
		}
	})

	t.Run("client setup error is propagated", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon().setupClient = func() (string, error) { return "", errors.New("restart failed") }
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.applyDaemonSetup(application.ModeClient, true)
		if err == nil || !strings.Contains(err.Error(), "restart failed") {
			t.Fatalf("expected setup error, got %v", err)
		}
	})

	t.Run("server restart success stores notice", func(t *testing.T) {
		status := systemd.UnitStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Role: systemd.UnitRoleClient}
		opts := defaultConfiguratorOpts()
		opts.Daemon = newDaemonControlStub()
		opts.testDaemon().status = func() (systemd.UnitStatus, error) { return status, nil }
		opts.testDaemon().setupServer = func() (string, error) {
			status.Role = systemd.UnitRoleServer
			status.ActiveState = "active"
			return "/etc/systemd/system/tungo.service", nil
		}
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated, err := model.applyDaemonSetup(application.ModeServer, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(updated.notice, "Server daemon reconfigured") {
			t.Fatalf("expected server reconfigure notice, got %q", updated.notice)
		}
		if updated.daemon.status.Role != systemd.UnitRoleServer {
			t.Fatalf("expected refreshed server role, got %+v", updated.daemon.status)
		}
	})

	t.Run("unknown mode returns explicit error", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.testDaemon()
		model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.applyDaemonSetup(0, false)
		if err == nil || !strings.Contains(err.Error(), "unknown daemon mode") {
			t.Fatalf("expected unknown daemon mode error, got %v", err)
		}
	})
}

func TestUpdateDaemonCheckErrorConfirmScreen_EscapeAndNavigation(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonCheckErrorConfirm
	model.pendingStartMode = application.ModeClient
	model.pendingStartScreen = configuratorScreenClientSelect

	updatedModel, cmd := model.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected navigation not to return a command")
	}
	if updated.cursor != 1 {
		t.Fatalf("expected cursor 1 after Down, got %d", updated.cursor)
	}

	updatedModel, cmd = updated.updateDaemonCheckErrorConfirmScreen(keyNamed(tea.KeyEsc))
	updated = updatedModel.(Configurator)
	if cmd != nil {
		t.Fatal("expected Escape not to return a command")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client selection, got %v", updated.screen)
	}
	if updated.notice != "Start cancelled." {
		t.Fatalf("expected cancellation notice, got %q", updated.notice)
	}
}

func TestStartModeWithDaemonGuard_PreservesNoticeOnStatusError(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.testDaemon().isActive = func() (bool, error) {
		return false, errors.New("status unavailable")
	}
	model, err := NewConfigurator(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.notice = "Configuration selected."

	updated := model.startModeWithDaemonGuard(
		application.ModeClient,
		configuratorScreenClientSelect,
		true,
	)
	if !strings.Contains(updated.notice, "Configuration selected.") ||
		!strings.Contains(updated.notice, "Failed to check daemon status: status unavailable") {
		t.Fatalf("expected original and status notices, got %q", updated.notice)
	}
	if updated.screen != configuratorScreenDaemonCheckErrorConfirm {
		t.Fatalf("expected status error confirmation, got %v", updated.screen)
	}
}

func TestDaemonPresentationHelpers_Boundaries(t *testing.T) {
	roleCases := []struct {
		execStart string
		want      string
	}{
		{execStart: "/usr/local/bin/tungo c", want: "client"},
		{execStart: "/usr/local/bin/tungo s", want: "server"},
		{execStart: "/usr/local/bin/tungo unknown", want: "unknown"},
	}
	for _, tc := range roleCases {
		if got := daemonRoleFromExecStart(tc.execStart); got != tc.want {
			t.Errorf("daemonRoleFromExecStart(%q) = %q, want %q", tc.execStart, got, tc.want)
		}
	}

	role, source := daemonDerivedRole(systemd.UnitStatus{}, "")
	if role != "unknown" || source != "Role" {
		t.Fatalf("daemonDerivedRole(empty) = %q, %q; want unknown, Role", role, source)
	}
	if isDaemonStartConfirmationScreen(configuratorScreenMode) {
		t.Fatal("mode screen must not be treated as a daemon confirmation")
	}
	if got := daemonSectionDivider(20); len(got) != 20 {
		t.Fatalf("daemonSectionDivider(20) length = %d, want 20", len(got))
	}
	if got := daemonMenuCursorAfterRefresh(nil, "", 4); got != 0 {
		t.Fatalf("empty menu cursor = %d, want 0", got)
	}
	if got := daemonMenuCursorAfterRefresh([]string{"start"}, "", -1); got != 0 {
		t.Fatalf("negative fallback cursor = %d, want 0", got)
	}
}

func TestMainTabView_DaemonConfirmScreens_ShowExpectedLabels(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = application.ModeClient
	clientView := model.mainTabView()
	if !strings.Contains(clientView, "requires restart") || !strings.Contains(clientView, "client daemon setup") {
		t.Fatalf("expected client reconfigure label in view, got: %s", clientView)
	}
	if !strings.Contains(clientView, "Tab switch tabs") {
		t.Fatalf("expected reconfigure confirm hint to include tab navigation, got: %s", clientView)
	}

	model.pendingDaemonMode = application.ModeServer
	serverView := model.mainTabView()
	if !strings.Contains(serverView, "server daemon setup") {
		t.Fatalf("expected server reconfigure label in view, got: %s", serverView)
	}

	model.screen = configuratorScreenDaemonActiveConfirm
	model.pendingStartMode = application.ModeClient
	startClientView := model.mainTabView()
	if !strings.Contains(startClientView, "starting client") {
		t.Fatalf("expected client start label in confirm view, got: %s", startClientView)
	}

	model.pendingStartMode = application.ModeServer
	startServerView := model.mainTabView()
	if !strings.Contains(startServerView, "starting server") {
		t.Fatalf("expected server start label in confirm view, got: %s", startServerView)
	}

	model.screen = configuratorScreenDaemonCheckErrorConfirm
	model.notice = "Failed to check daemon status: boom"
	checkErrorView := model.mainTabView()
	if !strings.Contains(checkErrorView, "Cannot verify daemon status") {
		t.Fatalf("expected check-error title in view, got: %s", checkErrorView)
	}
	if !strings.Contains(checkErrorView, retryDaemonCheckLabel) ||
		!strings.Contains(checkErrorView, startAnywayUnsafeLabel) ||
		!strings.Contains(checkErrorView, cancelLabel) {
		t.Fatalf("expected check-error options in view, got: %s", checkErrorView)
	}
}

func TestDaemonMenuOptions_DeactivatingStateShowsStopNotStart(t *testing.T) {
	model := Configurator{
		options: ConfiguratorOptions{
			Daemon: newDaemonControlStub(),
		},
	}
	options := model.daemonMenuOptions(systemd.UnitStatus{
		Installed:     true,
		ActiveState:   "deactivating",
		UnitFileState: "disabled",
		Role:          systemd.UnitRoleClient,
	})

	if !containsString(options, daemonStopLabel) {
		t.Fatalf("expected stop option for deactivating state, got %v", options)
	}
	if containsString(options, daemonStartLabel) {
		t.Fatalf("did not expect start option for deactivating state, got %v", options)
	}
}

func TestDaemonMenuOptions_StaticUnitFileDoesNotMapToEnableDisable(t *testing.T) {
	model := Configurator{
		options: ConfiguratorOptions{
			Daemon: newDaemonControlStub(),
		},
	}
	options := model.daemonMenuOptions(systemd.UnitStatus{
		Installed:     true,
		ActiveState:   "inactive",
		UnitFileState: "static",
		Role:          systemd.UnitRoleClient,
	})

	if containsString(options, daemonEnableLabel) || containsString(options, daemonDisableLabel) {
		t.Fatalf("did not expect enable/disable options for static unit-file state, got %v", options)
	}
}

func TestUpdateDaemonManageScreen_ReportsUnavailableActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
		notice string
	}{
		{name: "start", action: daemonStartLabel, notice: "Daemon start is unavailable."},
		{name: "stop", action: daemonStopLabel, notice: "Daemon stop is unavailable."},
		{name: "enable", action: daemonEnableLabel, notice: "Daemon enable is unavailable."},
		{name: "disable", action: daemonDisableLabel, notice: "Daemon disable is unavailable."},
		{name: "remove", action: daemonDeleteLabel, notice: "Daemon remove is unavailable."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := Configurator{
				daemon: daemonState{menuOptions: []string{tt.action}},
			}

			result, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
			updated := result.(Configurator)
			if cmd != nil {
				t.Fatal("expected no command")
			}
			if updated.notice != tt.notice {
				t.Fatalf("notice = %q, want %q", updated.notice, tt.notice)
			}
		})
	}
}

func TestDaemonMenuOptions_NoDaemonReturnsNil(t *testing.T) {
	if options := (Configurator{}).daemonMenuOptions(systemd.UnitStatus{}); options != nil {
		t.Fatalf("daemonMenuOptions() = %v, want nil", options)
	}
}

func containsString(values []string, want string) bool {
	return indexOfString(values, want) >= 0
}

func indexOfString(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}

func boolToActiveState(active bool) systemd.UnitActiveState {
	if active {
		return "active"
	}
	return "inactive"
}
