package bubble_tea

import (
	"errors"
	"strings"
	"testing"

	"tungo/domain/mode"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"

	tea "charm.land/bubbletea/v2"
)

func defaultConfiguratorOpts() ConfiguratorSessionOptions {
	return ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
		},
		ServerSupported: true,
	}
}

func settingsForMode(m ModePreference) *uiPreferencesProvider {
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = m
	return newUIPreferencesProvider(p)
}

// ---------------------------------------------------------------------------
// newConfiguratorSessionModel: auto-navigation based on AutoSelectMode
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AutoSelectModeClient_NavigatesToClientSelect(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectModeServer_NavigatesToServerSelect(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenServerSelect {
		t.Fatalf("expected configuratorScreenServerSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectModeNone_StaysAtModeScreen(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected configuratorScreenMode, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_ServerNotSupported_ModeNone_NavigatesToClientSelect(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerSupported = false

	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect when server unsupported with no preference, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_ServerNotSupported_ResetsServerModeToClient(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerSupported = false
	s := settingsForMode(ModePreferenceServer)

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reset: Server → Client, then auto-navigate to client select.
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect after server-mode reset, got %v", model.screen)
	}
	if s.Preferences().AutoSelectMode != ModePreferenceClient {
		t.Fatalf("expected AutoSelectMode reset to Client, got %q", s.Preferences().AutoSelectMode)
	}
}

func TestUpdateClientSelectScreen_Esc_ServerNotSupported_Exits(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerSupported = false
	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true on esc when server unsupported")
	}
	if !errors.Is(s.resultErr, ErrConfiguratorSessionUserExit) {
		t.Fatalf("expected ErrConfiguratorSessionUserExit, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
}

func TestUpdateClientSelectScreen_Esc_ServerSupported_GoesBackToModeScreen(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect

	result, _ := model.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected configuratorScreenMode on esc when server supported, got %v", s.screen)
	}
	if s.done {
		t.Fatal("expected done=false when server supported")
	}
}

func TestView_ClientSelectHint_ServerNotSupported_ShowsEscExit(t *testing.T) {
	opts := defaultConfiguratorOpts()
	opts.ServerSupported = false
	model, err := newConfiguratorSessionModel(opts, settingsForMode(ModePreferenceNone))
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
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.menuOptions = []string{sessionClientAdd}

	view := model.View().Content
	if !strings.Contains(view, "Esc back") {
		t.Fatalf("expected 'Esc back' in hint when server supported, got: %s", view)
	}
}

// ---------------------------------------------------------------------------
// updateClientSelectScreen: AutoSelectClientConfig saved only on success
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// newConfiguratorSessionModel: AutoSelectClientConfig skip logic
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_SkipsSelection(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	selector := &sessionSelectorRecorder{}
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.Selector = selector

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !model.done {
		t.Fatal("expected done=true when AutoSelectClientConfig matches an available config")
	}
	if model.resultMode != mode.Client {
		t.Fatalf("expected resultMode=Client, got %v", model.resultMode)
	}
	if selector.selected != "cfg.json" {
		t.Fatalf("expected selector to receive cfg.json, got %q", selector.selected)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_InvalidConfig_ShowsInvalidScreen(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.ClientConfigManager = sessionClientConfigManagerInvalid{
		err: errors.New("invalid client configuration (test): bad key"),
	}

	model, err := newConfiguratorSessionModel(opts, s)
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
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "cfg.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.ClientConfigManager = sessionClientConfigManagerNonInvalid{
		err: errors.New("permission denied"),
	}

	model, err := newConfiguratorSessionModel(opts, s)
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
	s := settingsForMode(ModePreferenceClient)
	p := s.Preferences()
	p.AutoConnect = true
	p.AutoSelectClientConfig = "missing.json"
	s.update(p)

	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"other.json"}}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.done {
		t.Fatal("expected done=false when AutoSelectClientConfig is missing from configs")
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select screen, got %v", model.screen)
	}
	if s.Preferences().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig reset to empty, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestNewConfiguratorSessionModel_AutoSelectClientConfig_NotSet_ShowsSelection(t *testing.T) {
	s := settingsForMode(ModePreferenceClient)

	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}

	model, err := newConfiguratorSessionModel(opts, s)
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
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().AutoSelectClientConfig != "cfg.json" {
		t.Fatalf("expected AutoSelectClientConfig=cfg.json, got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateClientSelectScreen_AutoSelectClientConfig_NotSavedWhenSelectFails(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.Selector = sessionSelectorFailStub{err: errors.New("select failed")}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged (empty), got %q", s.Preferences().AutoSelectClientConfig)
	}
}

func TestUpdateClientSelectScreen_AutoSelectClientConfig_NotSavedWhenConfigInvalid(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.ClientConfigManager = sessionClientConfigManagerInvalid{
		err: errors.New("invalid client configuration (test): bad key"),
	}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.client.configs = []string{"cfg.json"}
	model.client.menuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().AutoSelectClientConfig != "" {
		t.Fatalf("expected AutoSelectClientConfig unchanged (empty) after invalid config, got %q", s.Preferences().AutoSelectClientConfig)
	}
}
