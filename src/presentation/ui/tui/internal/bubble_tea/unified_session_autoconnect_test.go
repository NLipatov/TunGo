package bubble_tea

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tungo/application/runtime"
)

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

func TestTryAutoConnect_MissingClientConfigurationControl(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	if tryAutoConnect(prefs, ConfiguratorSessionOptions{}) {
		t.Fatal("expected false when client configuration control is nil")
	}
}

func TestTryAutoConnect_SelectFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	control := newTestConfigurationControl()
	control.selectErr = errors.New("select failed")
	opts := testSessionOptions(control)
	if tryAutoConnect(prefs, opts) {
		t.Fatal("expected false when Select returns error")
	}
}

func TestTryAutoConnect_ValidateActiveFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	control := newTestConfigurationControl()
	control.validateActiveErr = errors.New("bad config")
	opts := testSessionOptions(control)
	if tryAutoConnect(prefs, opts) {
		t.Fatal("expected false when active configuration validation fails")
	}
}

func TestTryAutoConnect_ValidationSucceeds(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	control := newTestConfigurationControl()
	opts := testSessionOptions(control)
	if !tryAutoConnect(prefs, opts) {
		t.Fatal("expected true when validation succeeds")
	}
}

func TestTryAutoConnect_AllSucceed(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prefs := UIPreferences{AutoSelectClientConfig: cfgPath}
	opts := testSessionOptions(newTestConfigurationControl())
	if !tryAutoConnect(prefs, opts) {
		t.Fatal("expected true when all conditions are met")
	}
}

func TestShouldDeferAutoConnectForDaemon_ActiveDaemon(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Daemon: &daemonControlStub{
			isActive: func() (bool, error) { return true, nil },
		},
	}
	if !shouldDeferAutoConnectForDaemon(opts) {
		t.Fatal("expected defer=true when daemon is active")
	}
}

func TestShouldDeferAutoConnectForDaemon_InactiveDaemon(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Daemon: &daemonControlStub{
			isActive: func() (bool, error) { return false, nil },
		},
	}
	if shouldDeferAutoConnectForDaemon(opts) {
		t.Fatal("expected defer=false when daemon is inactive")
	}
}

func TestShouldDeferAutoConnectForDaemon_NoHooks(t *testing.T) {
	if shouldDeferAutoConnectForDaemon(ConfiguratorSessionOptions{}) {
		t.Fatal("expected defer=false when daemon control are missing")
	}
}

func TestShouldDeferAutoConnectForDaemon_StatusCheckError(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Daemon: &daemonControlStub{
			isActive: func() (bool, error) { return false, errors.New("boom") },
		},
	}
	if !shouldDeferAutoConnectForDaemon(opts) {
		t.Fatal("expected defer=true when status check fails")
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
		if ev.kind != unifiedEventModeSelected || ev.mode != runtime.ModeClient {
			t.Fatalf("expected ModeSelected(Client), got kind=%d mode=%v", ev.kind, ev.mode)
		}
	default:
		t.Fatal("expected event in channel, got none")
	}
}

func TestNewUnifiedSessionModel_AutoConnect_DaemonActive_FallsBackToConfiguring(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	settings := settingsWithAutoConnect(cfgPath)
	events := make(chan unifiedEvent, 8)
	opts := defaultUnifiedConfigOpts()
	opts.testControl().clientConfigs = []string{cfgPath}
	opts.testDaemon().isActive = func() (bool, error) { return true, nil }
	opts.testDaemon().stop = func() error { return nil }

	m, err := newUnifiedSessionModel(context.Background(), opts, events, settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.phase != phaseConfiguring {
		t.Fatalf("expected phaseConfiguring when daemon is active, got %d", m.phase)
	}
	if m.configurator.screen != configuratorScreenDaemonActiveConfirm {
		t.Fatalf("expected daemon confirmation screen, got %v", m.configurator.screen)
	}
	if !settings.Preferences().AutoConnect {
		t.Fatal("expected AutoConnect to remain true while waiting for user confirmation")
	}
	select {
	case ev := <-events:
		t.Fatalf("expected no mode-selected event while daemon active, got kind=%d mode=%v", ev.kind, ev.mode)
	default:
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
// newUnifiedSessionModel: auto-connect when no server control is registered.
// ---------------------------------------------------------------------------

func serverUnsupportedOpts() ConfiguratorSessionOptions {
	opts := defaultUnifiedConfigOpts()
	opts.ServerConfigurationControl = nil
	return opts
}

// settingsWithAutoConnectNoMode sets AutoConnect=true with AutoSelectMode=None.
// Used to verify that a missing server control is sufficient to imply client mode.
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
		t.Fatalf("expected phaseWaitingForRuntime without server control and with AutoConnect, got %d", m.phase)
	}
	select {
	case ev := <-events:
		if ev.kind != unifiedEventModeSelected || ev.mode != runtime.ModeClient {
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
	// Saved preference is Server, but no server control is registered.
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
		if ev.kind != unifiedEventModeSelected || ev.mode != runtime.ModeClient {
			t.Fatalf("expected ModeSelected(Client), got kind=%d mode=%v", ev.kind, ev.mode)
		}
	default:
		t.Fatal("expected ModeSelected event in channel, got none")
	}
}
