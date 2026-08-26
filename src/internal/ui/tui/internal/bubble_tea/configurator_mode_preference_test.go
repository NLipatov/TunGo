package bubble_tea

import (
	"errors"
	"strings"
	"testing"

	tuiconfig "tungo/internal/config/tui"
	"tungo/internal/mode"

	tea "charm.land/bubbletea/v2"
)

func defaultConfiguratorOpts() ConfiguratorOptions {
	return testConfiguratorOptions(newTestConfigurationControl())
}

func settingsForMode(m tuiconfig.ModePreference) *Preferences {
	p := tuiconfig.Default()
	p.AutoSelectMode = m
	return newPreferences(p)
}

// ---------------------------------------------------------------------------
// NewConfigurator: auto-navigation based on AutoSelectMode
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AutoSelectModeClient_NavigatesToClientSelect(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(tuiconfig.ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
	if !strings.Contains(model.notice, "Auto-selected mode: client.") {
		t.Fatalf("expected autoselect mode notice, got %q", model.notice)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectModeServer_NavigatesToServerSelect(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(tuiconfig.ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenServerSelect {
		t.Fatalf("expected configuratorScreenServerSelect, got %v", model.screen)
	}
	if !strings.Contains(model.notice, "Auto-selected mode: server.") {
		t.Fatalf("expected autoselect mode notice, got %q", model.notice)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectModeNone_StaysAtModeScreen(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected configuratorScreenMode, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_ServerNotSupported_ModeNone_NavigatesToClientSelect(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurations = nil

	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect when server unsupported with no preference, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_ServerNotSupported_ResetsServerModeToClient(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurations = nil
	s := settingsForMode(tuiconfig.ModePreferenceServer)

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reset: Server → Client, then auto-navigate to client select.
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect after server-mode reset, got %v", model.screen)
	}
	if s.Current().AutoSelectMode != tuiconfig.ModePreferenceClient {
		t.Fatalf("expected AutoSelectMode reset to Client, got %q", s.Current().AutoSelectMode)
	}
}

func TestUpdateClientSelectScreen_Esc_ServerNotSupported_Exits(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurations = nil
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(Configurator)
	if !s.done {
		t.Fatal("expected done=true on esc when server unsupported")
	}
	if !errors.Is(s.resultErr, ErrConfiguratorUserExit) {
		t.Fatalf("expected ErrConfiguratorUserExit, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
}

func TestUpdateClientSelectScreen_Esc_ServerSupported_GoesBackToModeScreen(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect

	result, _ := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(Configurator)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected configuratorScreenMode on esc when server supported, got %v", s.screen)
	}
	if s.done {
		t.Fatal("expected done=false when server supported")
	}
}

func TestView_ClientSelectHint_ServerNotSupported_ShowsEscExit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerConfigurations = nil
	model, err := NewConfigurator(opts, settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	view := model.View().Content
	if !strings.Contains(view, "Esc exit") {
		t.Fatalf("expected 'Esc exit' in hint when server unsupported, got: %s", view)
	}
	if strings.Contains(view, "Esc back") {
		t.Fatalf("expected no 'Esc back' in hint when server unsupported, got: %s", view)
	}
}

func TestView_ClientSelectHint_ServerSupported_ShowsEscBack(t *testing.T) {
	model, err := NewConfigurator(defaultConfiguratorOpts(), settingsForMode(tuiconfig.ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.menuOptions = []string{clientAddLabel}

	view := model.View().Content
	if !strings.Contains(view, "Esc back") {
		t.Fatalf("expected 'Esc back' in hint when server supported, got: %s", view)
	}
}

// ---------------------------------------------------------------------------
// updateClientSelectScreen: AutoSelectClientConfig saved only on success
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// NewConfigurator: AutoSelectClientConfig skip logic
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_SkipsSelection(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !model.done {
		t.Fatal("expected done=true when AutoSelectClientConfig matches an available config")
	}
	if model.resultMode != mode.Client {
		t.Fatalf("expected resultMode=Client, got %v", model.resultMode)
	}
	if opts.testControl().activated != "cfg.json" {
		t.Fatalf("expected Activate to receive cfg.json, got %q", opts.testControl().activated)
	}
	if !strings.Contains(model.notice, "Auto-selected mode: client.") {
		t.Fatalf("expected autoselect mode notice, got %q", model.notice)
	}
	if !strings.Contains(model.notice, "Auto-selected config: cfg.json.") {
		t.Fatalf("expected autoselect config notice, got %q", model.notice)
	}
}

func TestNewConfiguratorSessionModel_MigratesSavedConfigurationPath(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "/etc/tungo/client_configuration.json.prod.office"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"office", "prod.office"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatal(err)
	}
	if !model.done {
		t.Fatal("expected the saved configuration to start")
	}
	if opts.testControl().activated != "prod.office" {
		t.Fatalf("Activate received %q, want prod.office", opts.testControl().activated)
	}
	if got := s.Current().AutoSelectClientConfig; got != "prod.office" {
		t.Fatalf("saved configuration = %q, want prod.office", got)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_DaemonActive_RequiresConfirmation(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}
	opts.testDaemon().isActive = func() (bool, error) { return true, nil }
	opts.testDaemon().stop = func() error { return nil }

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when daemon is active")
	}
	if model.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected configuratorScreenDaemonActiveConfirm, got %v", model.screen)
	}
	if model.pendingStartMode != mode.Client {
		t.Fatalf("expected pendingStartMode=Client, got %v", model.pendingStartMode)
	}
	if model.pendingClientConfig != "cfg.json" {
		t.Fatalf("expected pendingClientConfig=cfg.json, got %q", model.pendingClientConfig)
	}
	if opts.testControl().activated != "cfg.json" {
		t.Fatalf("expected Activate to receive cfg.json, got %q", opts.testControl().activated)
	}
	if !strings.Contains(model.notice, "Auto-selected mode: client.") {
		t.Fatalf("expected autoselect mode notice, got %q", model.notice)
	}
	if !strings.Contains(model.notice, "Auto-selected config: cfg.json.") {
		t.Fatalf("expected autoselect config notice, got %q", model.notice)
	}
}

// TestNewConfiguratorSessionModel_AutoConnect_False_AutoSelectClientConfig_Set_ShowsClientSelect is the
// direct regression test for the "auto-connects even with AutoConnect=false" bug. If AutoConnect is
// false, the auto-skip block must not run, even when AutoSelectClientConfig is set and valid.
func TestNewConfiguratorSessionModel_AutoConnect_False_AutoSelectClientConfig_Set_ShowsClientSelect(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = false
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when AutoConnect=false, even if AutoSelectClientConfig is set")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_ServerNotSupported_AutoConnect_False_AutoSelectClientConfig_Set_ShowsClientSelect(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceNone)
	p := s.Current()
	p.AutoConnect = false
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.ServerConfigurations = nil
	opts.testControl().clientConfigs = []string{"cfg.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when AutoConnect=false and !serverSupported")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_InvalidConfig_ShowsInvalidScreen(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}
	opts.testControl().activateErr = errors.New("invalid client configuration (test): bad key")

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when config is invalid")
	}
	if model.screen != configuratorScreenClientInvalid {
		t.Fatalf("expected configuratorScreenClientInvalid, got %v", model.screen)
	}
	if model.client.invalidConfig != "cfg.json" {
		t.Fatalf("expected invalidConfig=cfg.json, got %q", model.client.invalidConfig)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_NonInvalidError_ShowsNotice(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}
	opts.testControl().activateErr = errors.New("permission denied")

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when config manager returns non-invalid error")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
	if model.notice == "" {
		t.Fatal("expected notice to be set for non-invalid config error")
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_MissingConfig_ShowsSelection(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)
	p := s.Current()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "missing.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"other.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when AutoSelectClientConfig is missing from configs")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select screen, got %v", model.screen)
	}
	if s.Current().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig reset to empty, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_NotSet_ShowsSelection(t *testing.T) {
	s := settingsForMode(tuiconfig.ModePreferenceClient)

	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when AutoSelectClientConfig is not set")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select screen, got %v", model.screen)
	}
}

// ---------------------------------------------------------------------------
// updateClientSelectScreen: AutoSelectClientConfig saved only on success
// ---------------------------------------------------------------------------

func TestUpdateClientSelectScreen_AutoSelectClientConfig_SavedOnSuccess(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", clientRemoveLabel, clientAddLabel}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Current().AutoSelectClientConfig != "cfg.json" {
		t.Fatalf("expected AutoSelectClientConfig=cfg.json, got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateClientSelectScreen_AutoSelectClientConfig_NotSavedWhenActivateFails(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}
	opts.testControl().activateErr = errors.New("activate failed")

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", clientRemoveLabel, clientAddLabel}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Current().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged (empty), got %q", s.Current().AutoSelectClientConfig)
	}
}

func TestUpdateClientSelectScreen_AutoSelectClientConfig_NotSavedWhenConfigInvalid(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.testControl().clientConfigs = []string{"cfg.json"}
	opts.testControl().activateErr = errors.New("invalid client configuration (test): bad key")

	model, err := NewConfigurator(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", clientRemoveLabel, clientAddLabel}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Current().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged (empty) after invalid config, got %q", s.Current().AutoSelectClientConfig)
	}
}
