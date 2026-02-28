package bubble_tea

import (
	"errors"
	"testing"

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
	p.PreferredMode = m
	return newUIPreferencesProvider(p)
}

// ---------------------------------------------------------------------------
// newConfiguratorSessionModel: auto-navigation based on PreferredMode
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_PreferredModeClient_NavigatesToClientSelect(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenClientSelect {
		t.Fatalf("expected configuratorScreenClientSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_PreferredModeServer_NavigatesToServerSelect(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceServer))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenServerSelect {
		t.Fatalf("expected configuratorScreenServerSelect, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_PreferredModeNone_StaysAtModeScreen(t *testing.T) {
	model, err := newConfiguratorSessionModel(defaultConfiguratorOpts(), settingsForMode(ModePreferenceNone))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected configuratorScreenMode, got %v", model.screen)
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
	if s.Preferences().PreferredMode != ModePreferenceClient {
		t.Fatalf("expected PreferredMode reset to Client, got %q", s.Preferences().PreferredMode)
	}
}

// ---------------------------------------------------------------------------
// updateClientSelectScreen: LastClientConfig saved only on success
// ---------------------------------------------------------------------------

func TestUpdateClientSelectScreen_LastClientConfig_SavedOnSuccess(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientConfigs = []string{"cfg.json"}
	model.clientMenuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().LastClientConfig != "cfg.json" {
		t.Fatalf("expected LastClientConfig=cfg.json, got %q", s.Preferences().LastClientConfig)
	}
}

func TestUpdateClientSelectScreen_LastClientConfig_NotSavedWhenSelectFails(t *testing.T) {
	s := testSettings()
	opts := defaultConfiguratorOpts()
	opts.Observer = sessionObserverWithConfigs{configs: []string{"cfg.json"}}
	opts.Selector = sessionSelectorFailStub{err: errors.New("select failed")}

	model, err := newConfiguratorSessionModel(opts, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientConfigs = []string{"cfg.json"}
	model.clientMenuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().LastClientConfig != "" {
		t.Fatalf("expected LastClientConfig unchanged (empty), got %q", s.Preferences().LastClientConfig)
	}
}

func TestUpdateClientSelectScreen_LastClientConfig_NotSavedWhenConfigInvalid(t *testing.T) {
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
	model.clientConfigs = []string{"cfg.json"}
	model.clientMenuOptions = []string{"cfg.json", sessionClientRemove, sessionClientAdd}
	model.cursor = 0

	model.updateClientSelectScreen(keyNamed(tea.KeyEnter))

	if s.Preferences().LastClientConfig != "" {
		t.Fatalf("expected LastClientConfig unchanged (empty) after invalid config, got %q", s.Preferences().LastClientConfig)
	}
}