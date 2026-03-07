package bubble_tea

import (
	"errors"
	"strings"
	"testing"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"

	tea "charm.land/bubbletea/v2"
)

type failingClientConfigManager struct {
	err error
}

func (m failingClientConfigManager) Configuration() (*clientConfiguration.Configuration, error) {
	return nil, m.err
}

func TestModeOptions_AddsDaemonWhenSupported(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{
			Installed:     true,
			UnitFileState: "enabled",
			ActiveState:   "inactive",
			Mode:          mode.Client,
		}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsString(model.modeOptions, sessionModeDaemon) {
		t.Fatalf("expected daemon option in mode screen, got %v", model.modeOptions)
	}
}

func TestModeOptions_DoesNotAddDaemonWhenUnsupported(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = false

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsString(model.modeOptions, sessionModeDaemon) {
		t.Fatalf("expected no daemon option when unsupported, got %v", model.modeOptions)
	}
}

func TestUpdateModeScreen_EnterOnDaemon_OpensDaemonManageScreen(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenMode
	model.cursor = indexOfString(model.modeOptions, sessionModeDaemon)
	if model.cursor < 0 {
		t.Fatalf("expected daemon option in mode options, got %v", model.modeOptions)
	}

	updatedModel, _ := model.updateModeScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
}

func TestUpdateClientSelectScreen_Esc_ServerUnsupportedWithDaemon_ReturnsToModeScreen(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerSupported = false
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen when daemon option exists, got %v", model.screen)
	}

	model.screen = configuratorScreenClientSelect
	updatedModel, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(configuratorSessionModel)
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
	opts.ServerSupported = false
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceNone))
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
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Mode: mode.Server}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
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
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{
			Installed:      true,
			LoadState:      "loaded",
			UnitFileState:  "enabled",
			ActiveState:    "inactive",
			SubState:       "dead",
			Result:         "success",
			ExecMainStatus: "0",
			ExecStart:      "/usr/local/bin/tungo s",
			Mode:           mode.Server,
		}, nil
	}
	opts.StartSystemdUnit = func() error { return nil }
	opts.DisableSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
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
}

func TestUpdateDaemonManageScreen_NotInstalled_ShowsSetupOptions(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.InstallServerSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	if !containsString(model.daemon.menuOptions, sessionDaemonSetupClient) {
		t.Fatalf("expected setup client option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, sessionDaemonSetupServer) {
		t.Fatalf("expected setup server option, got %v", model.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_Installed_ShowsReconfigureOptions(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{
			Installed:     true,
			UnitFileState: "disabled",
			ActiveState:   "inactive",
			Mode:          mode.Client,
		}, nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.InstallServerSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.RemoveSystemdUnit = func() error { return nil }
	opts.StartSystemdUnit = func() error { return nil }
	opts.EnableSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	if containsString(model.daemon.menuOptions, sessionDaemonSetupClient) {
		t.Fatalf("did not expect setup client option for installed daemon, got %v", model.daemon.menuOptions)
	}
	if containsString(model.daemon.menuOptions, sessionDaemonSetupServer) {
		t.Fatalf("did not expect setup server option for installed daemon, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, sessionDaemonReconfClient) {
		t.Fatalf("expected reconfigure client option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, sessionDaemonReconfServer) {
		t.Fatalf("expected reconfigure server option, got %v", model.daemon.menuOptions)
	}
	if !containsString(model.daemon.menuOptions, sessionDaemonDelete) {
		t.Fatalf("expected delete daemon option, got %v", model.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_SetupClient_InstallsUnit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	installCalls := 0
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) {
		installCalls++
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{sessionDaemonSetupClient}
	model.cursor = 0

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if installCalls != 1 {
		t.Fatalf("expected one install call, got %d", installCalls)
	}
	if strings.Contains(updated.notice, "daemon configured") || strings.Contains(updated.notice, "daemon reconfigured") {
		t.Fatalf("expected no setup/reconfigure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_SetupClient_FailsWhenDefaultConfigInvalid(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.ClientConfigManager = failingClientConfigManager{err: errors.New("invalid default config")}
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: false}, nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{sessionDaemonSetupClient}
	model.cursor = 0

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if !strings.Contains(updated.notice, "cannot setup client daemon") {
		t.Fatalf("expected validation notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureInactive_AppliesImmediately(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Mode: mode.Client}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	reconfigureCalls := 0
	opts.InstallServerSystemdUnit = func() (string, error) {
		reconfigureCalls++
		status.Mode = mode.Server
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.StartSystemdUnit = func() error { return nil }
	opts.EnableSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, sessionDaemonReconfServer)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", sessionDaemonReconfServer, model.daemon.menuOptions)
	}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if reconfigureCalls != 1 {
		t.Fatalf("expected one reconfigure call, got %d", reconfigureCalls)
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected to stay on daemon manage screen, got %v", updated.screen)
	}
	if strings.Contains(updated.notice, "daemon configured") || strings.Contains(updated.notice, "daemon reconfigured") {
		t.Fatalf("expected no setup/reconfigure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureActive_ShowsMandatoryConfirm(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Mode: mode.Server}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.StopSystemdUnit = func() error { return nil }
	opts.StartSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, sessionDaemonReconfClient)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", sessionDaemonReconfClient, model.daemon.menuOptions)
	}

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd while waiting confirmation")
	}
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected reconfigure confirm screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != mode.Client {
		t.Fatalf("expected pending daemon mode client, got %v", updated.pendingDaemonMode)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_Confirm_RestartsWithNewSetup(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Mode: mode.Server}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }

	callOrder := make([]string, 0, 3)
	opts.StopSystemdUnit = func() error {
		callOrder = append(callOrder, "stop")
		status.ActiveState = "inactive"
		return nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) {
		callOrder = append(callOrder, "install-client")
		status.Mode = mode.Client
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.StartSystemdUnit = func() error {
		callOrder = append(callOrder, "start")
		status.ActiveState = "active"
		return nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = mode.Client
	model.cursor = 0 // stop and restart

	updatedModel, cmd := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd on reconfigure")
	}
	if strings.Join(callOrder, ",") != "stop,install-client,start" {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	if updated.pendingDaemonMode != mode.Unknown {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Client daemon reconfigured") || !strings.Contains(updated.notice, "restarted") {
		t.Fatalf("expected restarted notice, got %q", updated.notice)
	}
	if updated.daemon.status.ActiveState != "active" || updated.daemon.status.Mode != mode.Client {
		t.Fatalf("expected refreshed daemon status (active client), got %+v", updated.daemon.status)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_Cancel_ReturnsToDaemonManage(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = mode.Server
	model.cursor = 1 // cancel

	updatedModel, cmd := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd on cancel")
	}
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != mode.Unknown {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if !strings.Contains(updated.notice, "Reconfigure cancelled") {
		t.Fatalf("expected cancellation notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_StartEnableDisableStopFlow(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Mode: mode.Client}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.StartSystemdUnit = func() error {
		status.ActiveState = "active"
		return nil
	}
	opts.StopSystemdUnit = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.EnableSystemdUnit = func() error {
		status.UnitFileState = "enabled"
		return nil
	}
	opts.DisableSystemdUnit = func() error {
		status.UnitFileState = "disabled"
		return nil
	}
	opts.StopSystemdUnit = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.RemoveSystemdUnit = func() error {
		status = SystemdDaemonStatus{}
		return nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()

	model.daemon.menuOptions = []string{sessionDaemonStart}
	model.cursor = 0
	next, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.ActiveState != "active" {
		t.Fatal("expected daemon to be active after start")
	}
	if indexOfString(model.daemon.menuOptions, sessionDaemonStop) < 0 {
		t.Fatalf("expected %q option after start, got %v", sessionDaemonStop, model.daemon.menuOptions)
	}
	if indexOfString(model.daemon.menuOptions, sessionDaemonStart) >= 0 {
		t.Fatalf("did not expect %q option after start, got %v", sessionDaemonStart, model.daemon.menuOptions)
	}

	model.daemon.menuOptions = []string{sessionDaemonEnable}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.UnitFileState != "enabled" {
		t.Fatal("expected daemon to be enabled")
	}

	model.daemon.menuOptions = []string{sessionDaemonDisable}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.UnitFileState == "enabled" {
		t.Fatal("expected daemon to be disabled")
	}

	model.daemon.menuOptions = []string{sessionDaemonStop}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.ActiveState == "active" {
		t.Fatal("expected daemon to be stopped")
	}
	if indexOfString(model.daemon.menuOptions, sessionDaemonStart) < 0 {
		t.Fatalf("expected %q option after stop, got %v", sessionDaemonStart, model.daemon.menuOptions)
	}
	if indexOfString(model.daemon.menuOptions, sessionDaemonStop) >= 0 {
		t.Fatalf("did not expect %q option after stop, got %v", sessionDaemonStop, model.daemon.menuOptions)
	}
	if model.notice != "" {
		t.Fatalf("expected no success notice after stop, got %q", model.notice)
	}
}

func TestUpdateDaemonManageScreen_StartPreservesActionCursorAfterRefresh(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Mode: mode.Client}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.StartSystemdUnit = func() error {
		status.ActiveState = "active"
		return nil
	}
	opts.StopSystemdUnit = func() error {
		status.ActiveState = "inactive"
		return nil
	}
	opts.DisableSystemdUnit = func() error { return nil }
	opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.RemoveSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{"dummy-before", sessionDaemonStart, "dummy-after"}
	model.cursor = 1

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if len(updated.daemon.menuOptions) == 0 {
		t.Fatalf("expected daemon options after refresh")
	}
	if updated.cursor != 1 {
		t.Fatalf("expected cursor to stay on same action slot, got %d; options=%v", updated.cursor, updated.daemon.menuOptions)
	}
}

func TestUpdateDaemonManageScreen_Delete_RemovesUnitAndRefreshesStatus(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Mode: mode.Server}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	removeCalls := 0
	opts.RemoveSystemdUnit = func() error {
		removeCalls++
		status = SystemdDaemonStatus{}
		return nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, sessionDaemonDelete)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", sessionDaemonDelete, model.daemon.menuOptions)
	}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if removeCalls != 1 {
		t.Fatalf("expected one remove call, got %d", removeCalls)
	}
	if updated.notice != "" {
		t.Fatalf("expected no success notice, got %q", updated.notice)
	}
	if updated.daemon.status.Installed {
		t.Fatalf("expected daemon to be removed, got %+v", updated.daemon.status)
	}
	if !containsString(updated.daemon.menuOptions, sessionDaemonSetupClient) {
		t.Fatalf("expected setup options after delete, got %v", updated.daemon.menuOptions)
	}
}

func TestUpdateClientSelectScreen_SelectConfig_ActiveDaemon_ShowsStopPrompt(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg-a"}}
	opts.CheckSystemdUnitActive = func() (bool, error) { return true, nil }
	opts.StopSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = 0 // cfg-a

	updatedModel, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd when daemon stop confirmation is required")
	}
	if updated.done {
		t.Fatal("expected configurator to stay open for daemon stop confirmation")
	}
	if updated.screen != configuratorScreenSystemdActiveConfirm {
		t.Fatalf("expected systemd confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != mode.Client {
		t.Fatalf("expected pending start mode client, got %v", updated.pendingStartMode)
	}
	if updated.pendingStartScreen != configuratorScreenClientSelect {
		t.Fatalf("expected pending start screen client select, got %v", updated.pendingStartScreen)
	}
	if updated.pendingClientConfig != "cfg-a" {
		t.Fatalf("expected pending client config cfg-a, got %q", updated.pendingClientConfig)
	}
	if s.Preferences().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged before confirmation, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateServerSelectScreen_Start_ActiveDaemon_ShowsStopPrompt(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.CheckSystemdUnitActive = func() (bool, error) { return true, nil }
	opts.StopSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = indexOfString(model.server.menuOptions, sessionServerStart)

	updatedModel, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd when daemon stop confirmation is required")
	}
	if updated.done {
		t.Fatal("expected configurator to stay open for daemon stop confirmation")
	}
	if updated.screen != configuratorScreenSystemdActiveConfirm {
		t.Fatalf("expected systemd confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != mode.Server {
		t.Fatalf("expected pending start mode server, got %v", updated.pendingStartMode)
	}
}

func TestUpdateSystemdActiveConfirmScreen_EnterStop_StopsDaemonAndStartsMode(t *testing.T) {
	stopCalls := 0
	opts := defaultConfiguratorOpts()
	opts.StopSystemdUnit = func() error {
		stopCalls++
		return nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Server
	model.pendingStartScreen = configuratorScreenServerSelect
	model.cursor = 0 // stop and continue

	updatedModel, cmd := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if stopCalls != 1 {
		t.Fatalf("expected one stop call, got %d", stopCalls)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
	if !updated.done {
		t.Fatal("expected done=true after stop and continue")
	}
	if updated.resultMode != mode.Server {
		t.Fatalf("expected mode.Server, got %v", updated.resultMode)
	}
}

func TestUpdateSystemdActiveConfirmScreen_Cancel_ReturnsToPreviousScreen(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	opts := defaultConfiguratorOpts()
	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "new-cfg"
	model.cursor = 1 // cancel

	updatedModel, cmd := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd on cancel")
	}
	if updated.done {
		t.Fatal("expected done=false on cancel")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select, got %v", updated.screen)
	}
	if updated.pendingStartMode != mode.Unknown {
		t.Fatalf("expected pending mode cleared, got %v", updated.pendingStartMode)
	}
	if updated.pendingClientConfig != "" {
		t.Fatalf("expected pending client config cleared, got %q", updated.pendingClientConfig)
	}
	if !strings.Contains(updated.notice, "cancelled") {
		t.Fatalf("expected cancellation notice, got %q", updated.notice)
	}
	if s.Preferences().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected AutoSelectClientConfig unchanged on cancel, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateSystemdActiveConfirmScreen_StopFails_ShowsNoticeAndReturns(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.StopSystemdUnit = func() error { return errors.New("stop failed") }
	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "new-cfg"
	model.cursor = 0 // stop and continue

	updatedModel, cmd := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd when stop fails")
	}
	if updated.done {
		t.Fatal("expected done=false when stop fails")
	}
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Failed to stop systemd daemon") {
		t.Fatalf("expected stop failure notice, got %q", updated.notice)
	}
	if s.Preferences().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected AutoSelectClientConfig unchanged on stop failure, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateSystemdActiveConfirmScreen_EnterStop_Client_PersistsAutoSelectConfig(t *testing.T) {
	stopCalls := 0
	s := settingsForMode(ModePreferenceClient)
	opts := defaultConfiguratorOpts()
	opts.StopSystemdUnit = func() error {
		stopCalls++
		return nil
	}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "cfg-a"
	model.cursor = 0

	updatedModel, cmd := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if stopCalls != 1 {
		t.Fatalf("expected one stop call, got %d", stopCalls)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
	if !updated.done {
		t.Fatal("expected done=true after stop and continue")
	}
	if updated.resultMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", updated.resultMode)
	}
	if s.Preferences().AutoSelectClientConfig != "cfg-a" {
		t.Fatalf("expected AutoSelectClientConfig persisted after confirmation, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateDaemonManageScreen_Esc_LeavesDaemonManageScreen(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "inactive", Mode: mode.Server}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.RemoveSystemdUnit = func() error { return nil }
	opts.StartSystemdUnit = func() error { return nil }
	opts.EnableSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.tab = configuratorTabLogs
	model.pendingDaemonMode = mode.Server

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen, got %v", updated.screen)
	}
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected main tab after leave, got %v", updated.tab)
	}
	if updated.pendingDaemonMode != mode.Unknown {
		t.Fatalf("expected pending daemon mode cleared, got %v", updated.pendingDaemonMode)
	}
	if updated.cursor != indexOfString(updated.modeOptions, sessionModeDaemon) {
		t.Fatalf("expected cursor on daemon mode option, got %d (options=%v)", updated.cursor, updated.modeOptions)
	}
}

func TestRefreshDaemonStatus_UnavailableAndError(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.daemon.status = SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Mode: mode.Server}
	model.daemon.menuOptions = []string{sessionDaemonStop}

	model.refreshDaemonStatus()
	if model.daemon.statusErr == nil || !strings.Contains(model.daemon.statusErr.Error(), "unavailable") {
		t.Fatalf("expected unavailable status error, got %v", model.daemon.statusErr)
	}
	if model.daemon.status.Installed || len(model.daemon.menuOptions) != 0 {
		t.Fatalf("expected daemon status/menu to be reset, got status=%+v menu=%v", model.daemon.status, model.daemon.menuOptions)
	}

	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return SystemdDaemonStatus{}, errors.New("status boom") }
	model, err = newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.refreshDaemonStatus()
	if model.daemon.statusErr == nil || model.daemon.statusErr.Error() != "status boom" {
		t.Fatalf("expected status boom, got %v", model.daemon.statusErr)
	}
}

func TestDaemonStatusLineAndNotice_ErrorAndEmptyNotice(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.daemon.statusErr = errors.New("probe failed")
	if got := model.daemonStatusLine(); !strings.Contains(got, "Status error: probe failed") {
		t.Fatalf("expected status error line, got %q", got)
	}

	model.daemon.statusErr = nil
	model.daemon.status = SystemdDaemonStatus{Installed: true, UnitFileState: "disabled", ActiveState: "active", Mode: mode.Client}
	model.notice = ""
	want := model.daemonStatusLine()
	if got := model.daemonNotice(); got != want {
		t.Fatalf("expected daemonNotice to return status line when notice empty, got %q want %q", got, want)
	}
}

func TestUpdateDaemonManageScreen_UnavailableActions_ShowNotice(t *testing.T) {
	cases := []struct {
		name    string
		option  string
		active  bool
		wantMsg string
	}{
		{name: "setup client unavailable", option: sessionDaemonSetupClient, wantMsg: "client daemon setup is unavailable"},
		{name: "setup server unavailable", option: sessionDaemonSetupServer, wantMsg: "server daemon setup is unavailable"},
		{name: "reconfigure client unavailable", option: sessionDaemonReconfClient, wantMsg: "client daemon setup is unavailable"},
		{name: "reconfigure server unavailable", option: sessionDaemonReconfServer, wantMsg: "server daemon setup is unavailable"},
		{name: "start unavailable", option: sessionDaemonStart, wantMsg: "Daemon start is unavailable."},
		{name: "stop unavailable", option: sessionDaemonStop, wantMsg: "Daemon stop is unavailable."},
		{name: "enable unavailable", option: sessionDaemonEnable, wantMsg: "Daemon enable is unavailable."},
		{name: "disable unavailable", option: sessionDaemonDisable, wantMsg: "Daemon disable is unavailable."},
		{name: "delete unavailable", option: sessionDaemonDelete, wantMsg: "Daemon remove is unavailable."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			opts.SystemdSupported = true
			opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
				return SystemdDaemonStatus{Installed: true, ActiveState: boolToActiveState(tc.active), Mode: mode.Client}, nil
			}

			model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			model.screen = configuratorScreenDaemonManage
			model.daemon.status.ActiveState = boolToActiveState(tc.active)
			model.daemon.menuOptions = []string{tc.option}
			model.cursor = 0

			updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
			updated := updatedModel.(configuratorSessionModel)
			if !strings.Contains(updated.notice, tc.wantMsg) {
				t.Fatalf("expected %q in notice, got %q", tc.wantMsg, updated.notice)
			}
		})
	}
}

func TestUpdateDaemonManageScreen_ActionFailures_ShowNotice(t *testing.T) {
	cases := []struct {
		name           string
		option         string
		active         bool
		configureHooks func(*ConfiguratorSessionOptions)
		wantMsg        string
	}{
		{
			name:   "setup client install fails",
			option: sessionDaemonSetupClient,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.InstallClientSystemdUnit = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup client daemon",
		},
		{
			name:   "setup server install fails",
			option: sessionDaemonSetupServer,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.InstallServerSystemdUnit = func() (string, error) { return "", errors.New("install failed") }
			},
			wantMsg: "failed to setup server daemon",
		},
		{
			name:   "start fails",
			option: sessionDaemonStart,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.StartSystemdUnit = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to start daemon: boom",
		},
		{
			name:   "stop fails",
			option: sessionDaemonStop,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.StopSystemdUnit = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to stop daemon: boom",
		},
		{
			name:   "enable fails",
			option: sessionDaemonEnable,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.EnableSystemdUnit = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to enable daemon: boom",
		},
		{
			name:   "disable fails",
			option: sessionDaemonDisable,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.DisableSystemdUnit = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to disable daemon: boom",
		},
		{
			name:   "delete fails",
			option: sessionDaemonDelete,
			configureHooks: func(opts *ConfiguratorSessionOptions) {
				opts.RemoveSystemdUnit = func() error { return errors.New("boom") }
			},
			wantMsg: "Failed to remove daemon: boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultConfiguratorOpts()
			opts.SystemdSupported = true
			opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
				return SystemdDaemonStatus{Installed: true, ActiveState: boolToActiveState(tc.active), Mode: mode.Client}, nil
			}
			tc.configureHooks(&opts)

			model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			model.screen = configuratorScreenDaemonManage
			model.daemon.status.ActiveState = boolToActiveState(tc.active)
			model.daemon.menuOptions = []string{tc.option}
			model.cursor = 0

			updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
			updated := updatedModel.(configuratorSessionModel)
			if !strings.Contains(updated.notice, tc.wantMsg) {
				t.Fatalf("expected %q in notice, got %q", tc.wantMsg, updated.notice)
			}
		})
	}
}

func TestUpdateDaemonManageScreen_UnknownOption_Noop(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: true}, nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.notice = "keep"
	model.daemon.menuOptions = []string{"unknown-action"}

	updatedModel, _ := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if updated.notice != "keep" {
		t.Fatalf("expected notice to stay unchanged, got %q", updated.notice)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_EscAndNonEnter(t *testing.T) {
	opts := defaultConfiguratorOpts()
	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = mode.Server
	model.cursor = 0

	updatedModel, _ := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected to stay on confirm screen on non-enter, got %v", updated.screen)
	}

	updatedModel, _ = updated.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEsc))
	updated = updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenDaemonManage {
		t.Fatalf("expected daemon manage screen on esc, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != mode.Unknown {
		t.Fatalf("expected pending mode cleared on esc, got %v", updated.pendingDaemonMode)
	}
	if !strings.Contains(updated.notice, "Reconfigure cancelled.") {
		t.Fatalf("expected cancel notice on esc, got %q", updated.notice)
	}
}

func TestUpdateDaemonReconfigureConfirmScreen_ConfirmServerError_ShowsNotice(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: true, ActiveState: "active", Mode: mode.Client}, nil
	}
	opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = mode.Server
	model.cursor = 0

	updatedModel, _ := model.updateDaemonReconfigureConfirmScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if !strings.Contains(updated.notice, "daemon stop is unavailable") {
		t.Fatalf("expected missing stop capability notice, got %q", updated.notice)
	}
}

func TestStopAndRestartWithServerSetup_CoversBranches(t *testing.T) {
	t.Run("stop unavailable", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithServerSetup()
		if err == nil || !strings.Contains(err.Error(), "daemon stop is unavailable") {
			t.Fatalf("expected stop unavailable error, got %v", err)
		}
	})

	t.Run("stop fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return errors.New("stop failed") }
		opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithServerSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to stop daemon before reconfigure") {
			t.Fatalf("expected stop failure error, got %v", err)
		}
	})

	t.Run("install fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallServerSystemdUnit = func() (string, error) { return "", errors.New("install failed") }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithServerSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to setup server daemon") {
			t.Fatalf("expected setup failure error, got %v", err)
		}
	})

	t.Run("start unavailable", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		notice, err := model.stopAndRestartWithServerSetup()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(notice, "Start is unavailable") {
			t.Fatalf("expected start unavailable notice, got %q", notice)
		}
	})

	t.Run("start fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		opts.StartSystemdUnit = func() error { return errors.New("start failed") }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithServerSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to restart daemon after reconfigure") {
			t.Fatalf("expected restart failure error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		opts.StartSystemdUnit = func() error { return nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		notice, err := model.stopAndRestartWithServerSetup()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(notice, "Server daemon reconfigured") || !strings.Contains(notice, "restarted") {
			t.Fatalf("expected success notice, got %q", notice)
		}
	})
}

func TestUpdateSystemdActiveConfirmScreen_EscAndStopUnavailable(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	model.pendingStartScreen = configuratorScreenClientSelect
	model.pendingClientConfig = "cfg-a"

	updatedModel, _ := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEsc))
	updated := updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenClientSelect {
		t.Fatalf("expected return to client select on esc, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Start cancelled.") {
		t.Fatalf("expected cancel notice on esc, got %q", updated.notice)
	}

	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Server
	model.pendingStartScreen = configuratorScreenServerSelect
	model.cursor = 0
	updatedModel, _ = model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyEnter))
	updated = updatedModel.(configuratorSessionModel)
	if updated.screen != configuratorScreenServerSelect {
		t.Fatalf("expected return to server select when stop unavailable, got %v", updated.screen)
	}
	if !strings.Contains(updated.notice, "Stopping daemon is unavailable.") {
		t.Fatalf("expected unavailable notice, got %q", updated.notice)
	}
}

func TestStartModeWithSystemdGuard_PreserveNotice(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.CheckSystemdUnitActive = func() (bool, error) { return true, nil }
	opts.StopSystemdUnit = func() error { return nil }
	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.notice = "keep me"

	updated := model.startModeWithSystemdGuard(mode.Server, configuratorScreenServerSelect, true)
	if updated.screen != configuratorScreenSystemdActiveConfirm {
		t.Fatalf("expected confirm screen, got %v", updated.screen)
	}
	if updated.notice != "keep me" {
		t.Fatalf("expected notice to be preserved, got %q", updated.notice)
	}
	if updated.pendingStartMode != mode.Server || updated.pendingStartScreen != configuratorScreenServerSelect {
		t.Fatalf("expected pending start to be set, got mode=%v screen=%v", updated.pendingStartMode, updated.pendingStartScreen)
	}
}

func TestPersistAutoSelectClientConfig_EmptyValueIgnored(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoSelectClientConfig = "old-cfg"
	s.update(p)

	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model = model.persistAutoSelectClientConfig("   ")
	if s.Preferences().AutoSelectClientConfig != "old-cfg" {
		t.Fatalf("expected old config to remain unchanged, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestStopAndRestartWithClientSetup_CoversBranches(t *testing.T) {
	t.Run("stop unavailable", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithClientSetup()
		if err == nil || !strings.Contains(err.Error(), "daemon stop is unavailable") {
			t.Fatalf("expected stop unavailable error, got %v", err)
		}
	})

	t.Run("stop fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return errors.New("stop failed") }
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithClientSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to stop daemon before reconfigure") {
			t.Fatalf("expected stop failure error, got %v", err)
		}
	})

	t.Run("install fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallClientSystemdUnit = func() (string, error) { return "", errors.New("install failed") }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithClientSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to setup client daemon") {
			t.Fatalf("expected setup failure error, got %v", err)
		}
	})

	t.Run("start unavailable", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		notice, err := model.stopAndRestartWithClientSetup()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(notice, "Start is unavailable") {
			t.Fatalf("expected start unavailable notice, got %q", notice)
		}
	})

	t.Run("start fails", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		opts.StartSystemdUnit = func() error { return errors.New("start failed") }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.stopAndRestartWithClientSetup()
		if err == nil || !strings.Contains(err.Error(), "failed to restart daemon after reconfigure") {
			t.Fatalf("expected restart failure error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.StopSystemdUnit = func() error { return nil }
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		opts.StartSystemdUnit = func() error { return nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		notice, err := model.stopAndRestartWithClientSetup()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(notice, "Client daemon reconfigured") || !strings.Contains(notice, "restarted") {
			t.Fatalf("expected success notice, got %q", notice)
		}
	})
}

func TestStartModeWithSystemdGuard_CoversBranches(t *testing.T) {
	t.Run("without hooks starts immediately", func(t *testing.T) {
		model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithSystemdGuard(mode.Server, configuratorScreenServerSelect, false)
		if !updated.done || updated.resultMode != mode.Server {
			t.Fatalf("expected immediate start, got done=%v mode=%v", updated.done, updated.resultMode)
		}
	})

	t.Run("status check error", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.CheckSystemdUnitActive = func() (bool, error) { return false, errors.New("status failed") }
		opts.StopSystemdUnit = func() error { return nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithSystemdGuard(mode.Server, configuratorScreenServerSelect, false)
		if updated.screen != configuratorScreenServerSelect {
			t.Fatalf("expected return screen after status failure, got %v", updated.screen)
		}
		if !strings.Contains(updated.notice, "Failed to check systemd daemon status") {
			t.Fatalf("expected status failure notice, got %q", updated.notice)
		}
	})

	t.Run("inactive daemon starts immediately", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.CheckSystemdUnitActive = func() (bool, error) { return false, nil }
		opts.StopSystemdUnit = func() error { return nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated := model.startModeWithSystemdGuard(mode.Client, configuratorScreenClientSelect, false)
		if !updated.done || updated.resultMode != mode.Client {
			t.Fatalf("expected immediate client start, got done=%v mode=%v", updated.done, updated.resultMode)
		}
	})

	t.Run("active daemon clears notice when not preserving", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.CheckSystemdUnitActive = func() (bool, error) { return true, nil }
		opts.StopSystemdUnit = func() error { return nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		model.notice = "temporary notice"
		updated := model.startModeWithSystemdGuard(mode.Server, configuratorScreenServerSelect, false)
		if updated.screen != configuratorScreenSystemdActiveConfirm {
			t.Fatalf("expected confirm screen, got %v", updated.screen)
		}
		if updated.notice != "" {
			t.Fatalf("expected notice to be cleared, got %q", updated.notice)
		}
	})
}

func TestLeaveDaemonManageScreen_WithoutDaemonModeOption_ResetsCursor(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.modeOptions = []string{sessionModeClient, sessionModeServer}
	model.cursor = 1

	updated := model.leaveDaemonManageScreen()
	if updated.cursor != 0 {
		t.Fatalf("expected cursor reset to 0 when daemon option missing, got %d", updated.cursor)
	}
}

func TestUpdateDaemonManageScreen_NonEnter_DoesNothing(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) {
		return SystemdDaemonStatus{Installed: true, UnitFileState: "disabled", ActiveState: "inactive", Mode: mode.Client}, nil
	}
	startCalls := 0
	opts.StartSystemdUnit = func() error {
		startCalls++
		return nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.daemon.menuOptions = []string{sessionDaemonStart}
	model.cursor = 0

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(configuratorSessionModel)
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
	status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Mode: mode.Client}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.InstallServerSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
	opts.StopSystemdUnit = func() error { return nil }
	opts.StartSystemdUnit = func() error { return nil }

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenDaemonManage
	model.refreshDaemonStatus()
	model.cursor = indexOfString(model.daemon.menuOptions, sessionDaemonReconfServer)
	if model.cursor < 0 {
		t.Fatalf("missing %q in %v", sessionDaemonReconfServer, model.daemon.menuOptions)
	}

	updatedModel, cmd := model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd while waiting confirmation")
	}
	if updated.screen != configuratorScreenDaemonReconfigureConfirm {
		t.Fatalf("expected reconfigure confirm screen, got %v", updated.screen)
	}
	if updated.pendingDaemonMode != mode.Server {
		t.Fatalf("expected pending daemon mode server, got %v", updated.pendingDaemonMode)
	}
}

func TestUpdateSystemdActiveConfirmScreen_NonEnter_DoesNothing(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	model.pendingStartScreen = configuratorScreenClientSelect
	model.cursor = 0

	updatedModel, cmd := model.updateSystemdActiveConfirmScreen(keyNamed(tea.KeyDown))
	updated := updatedModel.(configuratorSessionModel)
	if cmd != nil {
		t.Fatalf("expected nil cmd on non-enter, got %v", cmd)
	}
	if updated.screen != configuratorScreenSystemdActiveConfirm {
		t.Fatalf("expected to stay on confirm screen, got %v", updated.screen)
	}
	if updated.pendingStartMode != mode.Client {
		t.Fatalf("expected pending mode to remain unchanged, got %v", updated.pendingStartMode)
	}
}

func TestApplyDaemonSetup_RestartBranchesAndUnknownMode(t *testing.T) {
	t.Run("client restart propagates restart error", func(t *testing.T) {
		opts := defaultConfiguratorOpts()
		opts.InstallClientSystemdUnit = func() (string, error) { return "/etc/systemd/system/tungo.service", nil }
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.applyDaemonSetup(mode.Client, true)
		if err == nil || !strings.Contains(err.Error(), "daemon stop is unavailable") {
			t.Fatalf("expected restart error, got %v", err)
		}
	})

	t.Run("server restart success stores notice", func(t *testing.T) {
		status := SystemdDaemonStatus{Installed: true, UnitFileState: "enabled", ActiveState: "active", Mode: mode.Client}
		opts := defaultConfiguratorOpts()
		opts.SystemdSupported = true
		opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
		opts.StopSystemdUnit = func() error {
			status.ActiveState = "inactive"
			return nil
		}
		opts.InstallServerSystemdUnit = func() (string, error) {
			status.Mode = mode.Server
			return "/etc/systemd/system/tungo.service", nil
		}
		opts.StartSystemdUnit = func() error {
			status.ActiveState = "active"
			return nil
		}
		model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		updated, err := model.applyDaemonSetup(mode.Server, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(updated.notice, "Server daemon reconfigured") {
			t.Fatalf("expected server reconfigure notice, got %q", updated.notice)
		}
		if updated.daemon.status.Mode != mode.Server {
			t.Fatalf("expected refreshed server role, got %+v", updated.daemon.status)
		}
	})

	t.Run("unknown mode returns explicit error", func(t *testing.T) {
		model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = model.applyDaemonSetup(mode.Unknown, false)
		if err == nil || err.Error() != "unknown daemon mode" {
			t.Fatalf("expected unknown daemon mode error, got %v", err)
		}
	})
}

func TestMainTabView_DaemonConfirmScreens_ShowExpectedLabels(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model.screen = configuratorScreenDaemonReconfigureConfirm
	model.pendingDaemonMode = mode.Client
	clientView := model.mainTabView()
	if !strings.Contains(clientView, "requires restart") || !strings.Contains(clientView, "client daemon setup") {
		t.Fatalf("expected client reconfigure label in view, got: %s", clientView)
	}

	model.pendingDaemonMode = mode.Server
	serverView := model.mainTabView()
	if !strings.Contains(serverView, "server daemon setup") {
		t.Fatalf("expected server reconfigure label in view, got: %s", serverView)
	}

	model.screen = configuratorScreenSystemdActiveConfirm
	model.pendingStartMode = mode.Client
	startClientView := model.mainTabView()
	if !strings.Contains(startClientView, "starting client") {
		t.Fatalf("expected client start label in confirm view, got: %s", startClientView)
	}

	model.pendingStartMode = mode.Server
	startServerView := model.mainTabView()
	if !strings.Contains(startServerView, "starting server") {
		t.Fatalf("expected server start label in confirm view, got: %s", startServerView)
	}
}

func TestDaemonMenuOptions_DeactivatingStateShowsStopNotStart(t *testing.T) {
	model := configuratorSessionModel{
		options: ConfiguratorSessionOptions{
			StartSystemdUnit: func() error { return nil },
			StopSystemdUnit:  func() error { return nil },
		},
	}
	options := model.daemonMenuOptions(SystemdDaemonStatus{
		Installed:     true,
		ActiveState:   "deactivating",
		UnitFileState: "disabled",
		Mode:          mode.Client,
	})

	if !containsString(options, sessionDaemonStop) {
		t.Fatalf("expected stop option for deactivating state, got %v", options)
	}
	if containsString(options, sessionDaemonStart) {
		t.Fatalf("did not expect start option for deactivating state, got %v", options)
	}
}

func TestDaemonMenuOptions_StaticUnitFileDoesNotMapToEnableDisable(t *testing.T) {
	model := configuratorSessionModel{
		options: ConfiguratorSessionOptions{
			EnableSystemdUnit:  func() error { return nil },
			DisableSystemdUnit: func() error { return nil },
		},
	}
	options := model.daemonMenuOptions(SystemdDaemonStatus{
		Installed:     true,
		ActiveState:   "inactive",
		UnitFileState: "static",
		Mode:          mode.Client,
	})

	if containsString(options, sessionDaemonEnable) || containsString(options, sessionDaemonDisable) {
		t.Fatalf("did not expect enable/disable options for static unit-file state, got %v", options)
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

func boolToActiveState(active bool) string {
	if active {
		return "active"
	}
	return "inactive"
}
