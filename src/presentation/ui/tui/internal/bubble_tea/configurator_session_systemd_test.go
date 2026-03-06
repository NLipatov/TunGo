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
			Installed: true,
			Enabled:   true,
			Active:    false,
			Mode:      mode.Client,
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
			Installed: true,
			Enabled:   false,
			Active:    false,
			Mode:      mode.Client,
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
	if !strings.Contains(updated.notice, "Client daemon configured") {
		t.Fatalf("expected success notice, got %q", updated.notice)
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
	status := SystemdDaemonStatus{Installed: true, Enabled: false, Active: false, Mode: mode.Client}
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
	if !strings.Contains(updated.notice, "Server daemon reconfigured") {
		t.Fatalf("expected reconfigure notice, got %q", updated.notice)
	}
}

func TestUpdateDaemonManageScreen_ReconfigureActive_ShowsMandatoryConfirm(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, Enabled: true, Active: true, Mode: mode.Server}
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
	status := SystemdDaemonStatus{Installed: true, Enabled: true, Active: true, Mode: mode.Server}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }

	callOrder := make([]string, 0, 3)
	opts.StopSystemdUnit = func() error {
		callOrder = append(callOrder, "stop")
		status.Active = false
		return nil
	}
	opts.InstallClientSystemdUnit = func() (string, error) {
		callOrder = append(callOrder, "install-client")
		status.Mode = mode.Client
		return "/etc/systemd/system/tungo.service", nil
	}
	opts.StartSystemdUnit = func() error {
		callOrder = append(callOrder, "start")
		status.Active = true
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
	if !updated.daemon.status.Active || updated.daemon.status.Mode != mode.Client {
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
	status := SystemdDaemonStatus{Installed: true, Enabled: false, Active: false, Mode: mode.Client}
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.GetSystemdDaemonStatus = func() (SystemdDaemonStatus, error) { return status, nil }
	opts.StartSystemdUnit = func() error {
		status.Active = true
		return nil
	}
	opts.EnableSystemdUnit = func() error {
		status.Enabled = true
		return nil
	}
	opts.DisableSystemdUnit = func() error {
		status.Enabled = false
		return nil
	}
	opts.StopSystemdUnit = func() error {
		status.Active = false
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
	if !status.Active {
		t.Fatal("expected daemon to be active after start")
	}

	model.daemon.menuOptions = []string{sessionDaemonEnable}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if !status.Enabled {
		t.Fatal("expected daemon to be enabled")
	}

	model.daemon.menuOptions = []string{sessionDaemonDisable}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.Enabled {
		t.Fatal("expected daemon to be disabled")
	}

	model.daemon.menuOptions = []string{sessionDaemonStop}
	next, _ = model.updateDaemonManageScreen(keyNamed(tea.KeyEnter))
	model = next.(configuratorSessionModel)
	if status.Active {
		t.Fatal("expected daemon to be stopped")
	}
	if model.notice != "" {
		t.Fatalf("expected no success notice after stop, got %q", model.notice)
	}
}

func TestUpdateDaemonManageScreen_Delete_RemovesUnitAndRefreshesStatus(t *testing.T) {
	status := SystemdDaemonStatus{Installed: true, Enabled: true, Active: false, Mode: mode.Server}
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
