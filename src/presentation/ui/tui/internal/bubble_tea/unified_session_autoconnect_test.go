package bubble_tea

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tungo/domain/mode"
)

// sessionSelectorFailStub is a Selector stub that always returns an error from Select.
type sessionSelectorFailStub struct {
	err error
}

func (s sessionSelectorFailStub) Select(string) error { return s.err }

// ---------------------------------------------------------------------------
// tryAutoConnect
// ---------------------------------------------------------------------------

func TestTryAutoConnect_EmptyLastConfig(t *testing.T) {
	if tryAutoConnect(UIPreferences{}, ConfiguratorSessionOptions{}) {
		t.Fatal("expected false for empty AutoSelectClientConfig")
	}
}

func TestTryAutoConnect_FileNotFound(t *testing.T) {
	prefs := UIPreferences{AutoSelectClientConfig: "/nonexistent/path/cfg.json"}
	if tryAutoConnect(prefs, ConfiguratorSessionOptions{}) {
		t.Fatal("expected false when config file does not exist")
	}
}

func TestTryAutoConnect_NilSelector(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	if tryAutoConnect(prefs, ConfiguratorSessionOptions{Selector: nil}) {
		t.Fatal("expected false when Selector is nil")
	}
}

func TestTryAutoConnect_SelectFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	opts := ConfiguratorSessionOptions{
		Selector: sessionSelectorFailStub{err: errors.New("select failed")},
	}
	if tryAutoConnect(prefs, opts) {
		t.Fatal("expected false when Select returns error")
	}
}

func TestTryAutoConnect_ConfigManagerFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	opts := ConfiguratorSessionOptions{
		Selector:            sessionSelectorStub{},
		ClientConfigManager: sessionClientConfigManagerInvalid{err: errors.New("bad config")},
	}
	if tryAutoConnect(prefs, opts) {
		t.Fatal("expected false when ClientConfigManager returns error")
	}
}

func TestTryAutoConnect_NilConfigManager_Succeeds(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	opts := ConfiguratorSessionOptions{
		Selector:            sessionSelectorStub{},
		ClientConfigManager: nil,
	}
	if !tryAutoConnect(prefs, opts) {
		t.Fatal("expected true when ClientConfigManager is nil and all else succeeds")
	}
}

func TestTryAutoConnect_AllSucceed(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	opts := ConfiguratorSessionOptions{
		Selector:            sessionSelectorStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
	}
	if !tryAutoConnect(prefs, opts) {
		t.Fatal("expected true when all conditions are met")
	}
}

// ---------------------------------------------------------------------------
// newUnifiedSessionModel: auto-connect
// ---------------------------------------------------------------------------

func settingsWithAutoConnect(cfgPath string) *uiPreferencesProvider {
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = ModePreferenceClient
	p.AutoConnect = true
	p.AutoSelectClientConfig = cfgPath
	return newUIPreferencesProvider(p)
}

func TestNewUnifiedSessionModel_AutoConnect_Succeeds_StartsWaiting(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	settings := settingsWithAutoConnect(cfgPath)
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime, got %d", m.phase)
	}
	select {
	case ev := <-events:
		if ev.kind != unifiedEventModeSelected || ev.mode != mode.Client {
			t.Fatalf("expected ModeSelected(Client), got kind=%d mode=%v", ev.kind, ev.mode)
		}
	default:
		t.Fatal("expected event in channel, got none")
	}
}

func TestNewUnifiedSessionModel_AutoConnect_FileGone_FallsBackToConfiguring(t *testing.T) {
	settings := settingsWithAutoConnect("/nonexistent/path.json")
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring when config file is gone, got %d", m.phase)
	}
	if settings.Preferences().AutoConnect {
		t.Fatal("expected AutoConnect reset to false when config file is missing")
	}
}

func TestNewUnifiedSessionModel_AutoConnect_ModeNone_NoAutoConnect(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = ModePreferenceNone
	p.AutoConnect = true
	p.AutoSelectClientConfig = cfgPath
	settings := newUIPreferencesProvider(p)
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring for mode=None with serverSupported=true, got %d", m.phase)
	}
	select {
	case <-events:
		t.Fatal("expected no events when mode=None")
	default:
	}
}

func TestNewUnifiedSessionModel_AutoConnect_Disabled_NoAutoConnect(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = ModePreferenceClient
	p.AutoConnect = false
	p.AutoSelectClientConfig = cfgPath
	settings := newUIPreferencesProvider(p)
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), defaultUnifiedConfigOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring when AutoConnect=false, got %d", m.phase)
	}
}

// ---------------------------------------------------------------------------
// newUnifiedSessionModel: auto-connect when ServerSupported=false
// ---------------------------------------------------------------------------

func serverUnsupportedOpts() ConfiguratorSessionOptions {
	opts := defaultUnifiedConfigOpts()
	opts.ServerSupported = false
	return opts
}

// settingsWithAutoConnectNoMode sets AutoConnect=true with AutoSelectMode=None.
// Used to verify that !ServerSupported alone is sufficient to imply client mode.
func settingsWithAutoConnectNoMode(cfgPath string) *uiPreferencesProvider {
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = ModePreferenceNone
	p.AutoConnect = true
	p.AutoSelectClientConfig = cfgPath
	return newUIPreferencesProvider(p)
}

func TestNewUnifiedSessionModel_ServerNotSupported_AutoConnect_Triggers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	settings := settingsWithAutoConnectNoMode(cfgPath)
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), serverUnsupportedOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime when ServerSupported=false+AutoConnect, got %d", m.phase)
	}
	select {
	case ev := <-events:
		if ev.kind != unifiedEventModeSelected || ev.mode != mode.Client {
			t.Fatalf("expected ModeSelected(Client), got kind=%d mode=%v", ev.kind, ev.mode)
		}
	default:
		t.Fatal("expected ModeSelected event in channel, got none")
	}
}

func TestNewUnifiedSessionModel_ServerNotSupported_AutoConnect_FileGone_ResetsAutoConnect(t *testing.T) {
	settings := settingsWithAutoConnectNoMode("/nonexistent/path.json")
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), serverUnsupportedOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring when config file is gone, got %d", m.phase)
	}
	if settings.Preferences().AutoConnect {
		t.Fatal("expected AutoConnect reset to false when AutoSelectClientConfig file is missing")
	}
}

func TestNewUnifiedSessionModel_ServerNotSupported_SavedServerMode_AutoConnect_Triggers(t *testing.T) {
	// Saved preference is Server, but ServerSupported=false.
	// The preference should be reset to Client, and auto-connect should still trigger.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)
	p.AutoSelectMode = ModePreferenceServer
	p.AutoConnect = true
	p.AutoSelectClientConfig = cfgPath
	settings := newUIPreferencesProvider(p)
	events := make(chan unifiedEvent, 8)

	m, err := newUnifiedSessionModel(context.Background(), serverUnsupportedOpts(), events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseWaitingForRuntime {
		t.Fatalf("expected phaseWaitingForRuntime after server-mode reset + auto-connect, got %d", m.phase)
	}
	select {
	case ev := <-events:
		if ev.kind != unifiedEventModeSelected || ev.mode != mode.Client {
			t.Fatalf("expected ModeSelected(Client), got kind=%d mode=%v", ev.kind, ev.mode)
		}
	default:
		t.Fatal("expected ModeSelected event in channel, got none")
	}
}
