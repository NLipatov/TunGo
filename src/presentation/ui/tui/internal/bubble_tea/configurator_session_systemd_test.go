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

func TestReloadClientConfigs_AddsSystemdOptionWhenSupported(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.InstallClientSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(model.client.menuOptions) != 2 {
		t.Fatalf("expected two client options (daemon + add), got %v", model.client.menuOptions)
	}
	if model.client.menuOptions[0] != sessionClientDaemon || model.client.menuOptions[1] != sessionClientAdd {
		t.Fatalf("unexpected client options order: %v", model.client.menuOptions)
	}
}

func TestReloadClientConfigs_DoesNotAddSystemdOptionWhenUnsupported(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = false

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(model.client.menuOptions) != 1 || model.client.menuOptions[0] != sessionClientAdd {
		t.Fatalf("expected only add option when systemd unsupported, got %v", model.client.menuOptions)
	}
}

func TestUpdateClientSelectScreen_EnterOnSystemdOption_InstallsUnit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	installCalls := 0
	opts.InstallClientSystemdUnit = func() (string, error) {
		installCalls++
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = 0 // daemon option

	updatedModel, _ := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if installCalls != 1 {
		t.Fatalf("expected installer to be called once, got %d", installCalls)
	}
	if !strings.Contains(updated.notice, "systemctl start tungo.service") {
		t.Fatalf("expected post-install notice with start command, got %q", updated.notice)
	}
	if updated.done {
		t.Fatal("expected configurator to stay open after daemon setup")
	}
}

func TestUpdateClientSelectScreen_SystemdOption_FailsWhenDefaultConfigInvalid(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.ClientConfigManager = failingClientConfigManager{err: errors.New("invalid default config")}
	installCalls := 0
	opts.InstallClientSystemdUnit = func() (string, error) {
		installCalls++
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = 0 // daemon option

	updatedModel, _ := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if installCalls != 0 {
		t.Fatalf("expected installer not to be called when default config invalid, got %d", installCalls)
	}
	if !strings.Contains(updated.notice, "Cannot setup client daemon") {
		t.Fatalf("expected validation notice, got %q", updated.notice)
	}
}

func TestServerMenu_AddsSystemdOptionWhenSupported(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	opts.InstallServerSystemdUnit = func() (string, error) {
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsString(model.server.menuOptions, sessionServerDaemon) {
		t.Fatalf("expected server daemon option in menu, got %v", model.server.menuOptions)
	}
}

func TestUpdateServerSelectScreen_EnterOnSystemdOption_InstallsUnit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.SystemdSupported = true
	installCalls := 0
	opts.InstallServerSystemdUnit = func() (string, error) {
		installCalls++
		return "/etc/systemd/system/tungo.service", nil
	}

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.cursor = indexOfString(model.server.menuOptions, sessionServerDaemon)
	if model.cursor < 0 {
		t.Fatalf("server daemon option not found in %v", model.server.menuOptions)
	}

	updatedModel, _ := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	updated := updatedModel.(configuratorSessionModel)
	if installCalls != 1 {
		t.Fatalf("expected installer call once, got %d", installCalls)
	}
	if !strings.Contains(updated.notice, "systemctl start tungo.service") {
		t.Fatalf("expected post-install notice with start command, got %q", updated.notice)
	}
	if updated.done {
		t.Fatal("expected configurator to stay open after daemon setup")
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
