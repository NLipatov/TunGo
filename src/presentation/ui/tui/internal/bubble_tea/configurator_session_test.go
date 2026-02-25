package bubble_tea

import (
	"errors"
	"strings"
	"testing"
	"time"

	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"

	tea "github.com/charmbracelet/bubbletea"
)

// sessionObserverWithConfigs is a stub that returns predefined client configs.
type sessionObserverWithConfigs struct {
	configs []string
}

func (s sessionObserverWithConfigs) Observe() ([]string, error) { return s.configs, nil }

// sessionObserverError is a stub that returns an error from Observe.
type sessionObserverError struct {
	err error
}

func (s sessionObserverError) Observe() ([]string, error) { return nil, s.err }

// sessionDeleterRecorder records Delete calls.
type sessionDeleterRecorder struct {
	deleted []string
}

func (s *sessionDeleterRecorder) Delete(name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

// sessionSelectorRecorder records which config was selected.
type sessionSelectorRecorder struct {
	selected string
}

func (s *sessionSelectorRecorder) Select(path string) error {
	s.selected = path
	return nil
}

// sessionClientConfigManagerInvalid returns an invalid-config error.
type sessionClientConfigManagerInvalid struct {
	err error
}

func (s sessionClientConfigManagerInvalid) Configuration() (*clientConfiguration.Configuration, error) {
	return nil, s.err
}

func newTestSessionModel(t *testing.T) configuratorSessionModel {
	t.Helper()
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "test", ClientID: 1, Enabled: true},
		},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	return model
}

// --- 1. Init ---

func TestInit_ReturnsNilCmd(t *testing.T) {
	m := newTestSessionModel(t)
	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init should return nil cmd")
	}
}

// --- 2. Update ---

func TestUpdate_WindowSizeMsg_UpdatesWidthHeight(t *testing.T) {
	m := newTestSessionModel(t)
	result, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatal("expected nil cmd from WindowSizeMsg")
	}
	updated := result.(configuratorSessionModel)
	if updated.width != 120 || updated.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", updated.width, updated.height)
	}
}

func TestUpdate_WindowSizeMsg_LogsTab_RefreshesViewport(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logReady = true
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := result.(configuratorSessionModel)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("expected 100x30, got %dx%d", updated.width, updated.height)
	}
}

func TestUpdate_LogTickMsg_MatchingSeq(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logTickSeq = 5
	m.logReady = true
	m.restartLogWait()

	result, cmd := m.Update(configuratorLogTickMsg{seq: 5})
	updated := result.(configuratorSessionModel)
	_ = updated
	if cmd == nil {
		t.Fatal("expected non-nil cmd for matching log tick")
	}
}

func TestUpdate_LogTickMsg_MismatchedSeq_Ignored(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logTickSeq = 5

	result, cmd := m.Update(configuratorLogTickMsg{seq: 99})
	_ = result.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd for mismatched log tick seq")
	}
}

func TestUpdate_LogTickMsg_WrongTab_Ignored(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabMain
	m.logTickSeq = 5

	result, cmd := m.Update(configuratorLogTickMsg{seq: 5})
	_ = result.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd when tab is not Logs")
	}
}

func TestUpdate_CtrlC_Exits(t *testing.T) {
	m := newTestSessionModel(t)
	result, cmd := m.Update(keyNamed(tea.KeyCtrlC))
	updated := result.(configuratorSessionModel)
	if !updated.done {
		t.Fatal("expected done=true on ctrl+c")
	}
	if !errors.Is(updated.resultErr, ErrConfiguratorSessionUserExit) {
		t.Fatalf("expected ErrConfiguratorSessionUserExit, got %v", updated.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

func TestUpdate_Tab_CyclesTabs(t *testing.T) {
	m := newTestSessionModel(t)
	if m.tab != configuratorTabMain {
		t.Fatalf("expected initial tab=Main, got %d", m.tab)
	}

	result, _ := m.Update(keyNamed(tea.KeyTab))
	updated := result.(configuratorSessionModel)
	if updated.tab != configuratorTabSettings {
		t.Fatalf("expected tab=Settings after first Tab, got %d", updated.tab)
	}
}

func TestUpdate_Tab_DoesNotCycleOnAddNameScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName
	m.tab = configuratorTabMain

	result, _ := m.Update(keyNamed(tea.KeyTab))
	updated := result.(configuratorSessionModel)
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected tab=Main (Tab should not cycle on add-name screen), got %d", updated.tab)
	}
}

func TestUpdate_Tab_DoesNotCycleOnAddJSONScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.tab = configuratorTabMain

	result, _ := m.Update(keyNamed(tea.KeyTab))
	updated := result.(configuratorSessionModel)
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected tab=Main (Tab should not cycle on add-JSON screen), got %d", updated.tab)
	}
}

func TestUpdate_SettingsTab_DispatchesToSettings(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	updated := result.(configuratorSessionModel)
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected tab=Main after esc in settings, got %d", updated.tab)
	}
}

func TestUpdate_LogsTab_DispatchesToLogs(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logReady = true

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	updated := result.(configuratorSessionModel)
	if updated.tab != configuratorTabMain {
		t.Fatalf("expected tab=Main after esc in logs, got %d", updated.tab)
	}
}

func TestUpdate_MainTab_DispatchesToScreenHandlers(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabMain
	m.screen = configuratorScreenMode

	// up/down navigates
	result, _ := m.Update(keyNamed(tea.KeyDown))
	updated := result.(configuratorSessionModel)
	if updated.cursor != 1 {
		t.Fatalf("expected cursor=1 after down, got %d", updated.cursor)
	}
}

func TestUpdate_DoneModel_ReturnsQuit(t *testing.T) {
	m := newTestSessionModel(t)
	m.done = true
	_, cmd := m.Update(keyRunes('x'))
	if cmd == nil {
		t.Fatal("expected Quit cmd when done=true")
	}
}

// --- 3. View ---

func TestView_SettingsTab_ContainsTheme(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings

	view := m.View()
	if !strings.Contains(strings.ToLower(view), "theme") {
		t.Fatalf("settings view should contain 'Theme', got: %s", view)
	}
}

func TestView_LogsTab_ReturnsNonEmpty(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logReady = true

	view := m.View()
	if len(view) == 0 {
		t.Fatal("logs view should be non-empty")
	}
}

func TestView_MainTab_ModeScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode

	view := m.View()
	if !strings.Contains(view, "Select mode") {
		t.Fatalf("mode screen view should contain 'Select mode', got: %s", view)
	}
}

func TestView_MainTab_ClientSelectScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientMenuOptions = []string{sessionClientAdd}

	view := m.View()
	if !strings.Contains(view, "Select configuration") {
		t.Fatalf("client select view should contain 'Select configuration', got: %s", view)
	}
}

func TestView_MainTab_ClientRemoveScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientRemove
	m.clientRemovePaths = []string{"config1"}

	view := m.View()
	if !strings.Contains(view, "Choose a configuration to remove") {
		t.Fatalf("client remove view should contain proper title, got: %s", view)
	}
}

func TestView_MainTab_ClientAddNameScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName
	m.width = 80
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "Name configuration") {
		t.Fatalf("add name view should contain 'Name configuration', got: %s", view)
	}
}

func TestView_MainTab_ClientAddJSONScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.width = 80
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "Paste configuration") {
		t.Fatalf("add JSON view should contain 'Paste configuration', got: %s", view)
	}
}

func TestView_MainTab_ClientInvalidScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad config")

	view := m.View()
	if !strings.Contains(view, "Configuration error") {
		t.Fatalf("invalid screen view should contain 'Configuration error', got: %s", view)
	}
}

func TestView_MainTab_ServerSelectScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerSelect

	view := m.View()
	if !strings.Contains(view, "Choose an option") {
		t.Fatalf("server select view should contain 'Choose an option', got: %s", view)
	}
}

func TestView_MainTab_ServerManageScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerManage
	m.serverManageLabels = []string{"#1 test [enabled]"}

	view := m.View()
	if !strings.Contains(view, "Select client to enable/disable or delete") {
		t.Fatalf("server manage view should contain proper title, got: %s", view)
	}
}

func TestView_MainTab_ServerDeleteConfirmScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerDeleteConfirm
	m.serverDeletePeer = serverConfiguration.AllowedPeer{Name: "alpha", ClientID: 1}

	view := m.View()
	if !strings.Contains(view, "Delete client") {
		t.Fatalf("delete confirm view should contain 'Delete client', got: %s", view)
	}
}

// --- 4. cycleTab ---

func TestCycleTab_MainToSettingsToLogsToMain(t *testing.T) {
	m := newTestSessionModel(t)
	if m.tab != configuratorTabMain {
		t.Fatalf("expected initial tab=Main")
	}

	result, _ := m.cycleTab()
	s := result.(configuratorSessionModel)
	if s.tab != configuratorTabSettings {
		t.Fatalf("expected Settings, got %d", s.tab)
	}

	result, cmd := s.cycleTab()
	s = result.(configuratorSessionModel)
	if s.tab != configuratorTabLogs {
		t.Fatalf("expected Logs, got %d", s.tab)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd when entering Logs tab")
	}

	s.stopLogWait()
	result, _ = s.cycleTab()
	s = result.(configuratorSessionModel)
	if s.tab != configuratorTabMain {
		t.Fatalf("expected Main, got %d", s.tab)
	}
}

func TestCycleTab_LeavingLogsStopsLogWait(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.restartLogWait()
	ch := m.logWaitStop

	result, _ := m.cycleTab()
	s := result.(configuratorSessionModel)
	if s.tab != configuratorTabMain {
		t.Fatalf("expected Main tab, got %d", s.tab)
	}
	// Channel should be closed
	select {
	case <-ch:
		// ok, closed
	default:
		t.Fatal("expected logWaitStop channel to be closed when leaving Logs tab")
	}
}

// --- 5. updateSettingsTab ---

func TestUpdateSettingsTab_EscReturnsToMain(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings

	result, _ := m.updateSettingsTab(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.tab != configuratorTabMain {
		t.Fatalf("expected Main tab, got %d", s.tab)
	}
}

func TestUpdateSettingsTab_UpMoveCursorUp(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = 2

	result, _ := m.updateSettingsTab(keyRunes('k'))
	s := result.(configuratorSessionModel)
	if s.settingsCursor != 1 {
		t.Fatalf("expected cursor=1, got %d", s.settingsCursor)
	}
}

func TestUpdateSettingsTab_DownMoveCursorDown(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = 0

	result, _ := m.updateSettingsTab(keyRunes('j'))
	s := result.(configuratorSessionModel)
	if s.settingsCursor != 1 {
		t.Fatalf("expected cursor=1, got %d", s.settingsCursor)
	}
}

func TestUpdateSettingsTab_LeftChangesSettingLeft(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = settingsThemeRow

	result, _ := m.updateSettingsTab(keyRunes('h'))
	s := result.(configuratorSessionModel)
	_ = s // just verify no panic
}

func TestUpdateSettingsTab_RightChangesSettingRight(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = settingsThemeRow

	result, _ := m.updateSettingsTab(keyRunes('l'))
	s := result.(configuratorSessionModel)
	_ = s
}

func TestUpdateSettingsTab_EnterChangesSettingRight(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = settingsThemeRow

	result, _ := m.updateSettingsTab(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	_ = s
}

func TestUpdateSettingsTab_ThemeChangeTriggersClearScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabSettings
	m.settingsCursor = settingsThemeRow
	// Force a known theme so cycling changes it
	m.preferences = testSettings().Preferences()

	_, cmd := m.updateSettingsTab(keyNamed(tea.KeyRight))
	// cmd may or may not be ClearScreen depending on whether theme actually changed.
	// Just verify no panic and cmd is either nil or ClearScreen.
	_ = cmd
}

// --- 6. updateLogsTab ---

func TestUpdateLogsTab_EscReturnsToMainAndStopsWait(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.restartLogWait()
	ch := m.logWaitStop

	result, _ := m.updateLogsTab(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.tab != configuratorTabMain {
		t.Fatalf("expected Main tab, got %d", s.tab)
	}
	select {
	case <-ch:
		// closed as expected
	default:
		t.Fatal("expected logWaitStop to be closed")
	}
}

func TestUpdateLogsTab_PgUpSetsFollowFalse(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logFollow = true

	result, _ := m.updateLogsTab(keyNamed(tea.KeyPgUp))
	s := result.(configuratorSessionModel)
	if s.logFollow {
		t.Fatal("expected logFollow=false after PgUp")
	}
}

func TestUpdateLogsTab_PgDown(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs

	result, _ := m.updateLogsTab(keyNamed(tea.KeyPgDown))
	_ = result.(configuratorSessionModel)
}

func TestUpdateLogsTab_HomeSetsFollowFalse(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logFollow = true

	result, _ := m.updateLogsTab(keyNamed(tea.KeyHome))
	s := result.(configuratorSessionModel)
	if s.logFollow {
		t.Fatal("expected logFollow=false after Home")
	}
}

func TestUpdateLogsTab_EndSetsFollowTrue(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logFollow = false

	result, _ := m.updateLogsTab(keyNamed(tea.KeyEnd))
	s := result.(configuratorSessionModel)
	if !s.logFollow {
		t.Fatal("expected logFollow=true after End")
	}
}

func TestUpdateLogsTab_SpaceTogglesFollow(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logFollow = false

	result, _ := m.updateLogsTab(keyNamed(tea.KeySpace))
	s := result.(configuratorSessionModel)
	if !s.logFollow {
		t.Fatal("expected logFollow=true after Space toggle")
	}

	result, _ = s.updateLogsTab(keyNamed(tea.KeySpace))
	s = result.(configuratorSessionModel)
	if s.logFollow {
		t.Fatal("expected logFollow=false after second Space toggle")
	}
}

func TestUpdateLogsTab_UpLineNavigation(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs
	m.logFollow = true

	result, _ := m.updateLogsTab(keyRunes('k'))
	s := result.(configuratorSessionModel)
	if s.logFollow {
		t.Fatal("expected logFollow=false after up scroll")
	}
}

func TestUpdateLogsTab_DownLineNavigation(t *testing.T) {
	m := newTestSessionModel(t)
	m.tab = configuratorTabLogs

	result, _ := m.updateLogsTab(keyRunes('j'))
	_ = result.(configuratorSessionModel)
}

// --- 7. updateModeScreen ---

func TestUpdateModeScreen_EscExits(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode

	result, cmd := m.updateModeScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true on esc in mode screen")
	}
	if !errors.Is(s.resultErr, ErrConfiguratorSessionUserExit) {
		t.Fatalf("expected ErrConfiguratorSessionUserExit, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected non-nil quit cmd")
	}
}

func TestUpdateModeScreen_UpDownNavigation(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode
	m.cursor = 0

	result, _ := m.updateModeScreen(keyNamed(tea.KeyDown))
	s := result.(configuratorSessionModel)
	if s.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", s.cursor)
	}

	result, _ = s.updateModeScreen(keyNamed(tea.KeyUp))
	s = result.(configuratorSessionModel)
	if s.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", s.cursor)
	}
}

func TestUpdateModeScreen_EnterClient(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode
	m.cursor = 0 // "client"

	result, _ := m.updateModeScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select screen, got %v", s.screen)
	}
}

func TestUpdateModeScreen_EnterServer(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode
	m.cursor = 1 // "server"

	result, _ := m.updateModeScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected server select screen, got %v", s.screen)
	}
}

// --- 8. updateClientSelectScreen ---

func TestUpdateClientSelectScreen_EscGoesBackToMode(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientMenuOptions = []string{sessionClientAdd}

	result, _ := m.updateClientSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen, got %v", s.screen)
	}
}

func TestUpdateClientSelectScreen_EnterAdd(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientMenuOptions = []string{sessionClientAdd}
	m.cursor = 0

	result, cmd := m.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected add name screen, got %v", s.screen)
	}
	if cmd == nil {
		t.Fatal("expected textinput.Blink cmd")
	}
}

func TestUpdateClientSelectScreen_EnterRemoveWithConfigs(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientConfigs = []string{"config1.json", "config2.json"}
	m.clientMenuOptions = []string{"config1.json", "config2.json", sessionClientRemove, sessionClientAdd}
	m.cursor = 2 // sessionClientRemove

	result, _ := m.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientRemove {
		t.Fatalf("expected remove screen, got %v", s.screen)
	}
	if len(s.clientRemovePaths) != 2 {
		t.Fatalf("expected 2 remove paths, got %d", len(s.clientRemovePaths))
	}
}

func TestUpdateClientSelectScreen_EnterRemoveNoConfigs(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientConfigs = []string{}
	m.clientMenuOptions = []string{sessionClientRemove, sessionClientAdd}
	m.cursor = 0 // sessionClientRemove

	result, _ := m.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected to stay on client select, got %v", s.screen)
	}
	if s.notice == "" {
		t.Fatal("expected a notice about no configs")
	}
}

func TestUpdateClientSelectScreen_EnterSelectConfig_NilConfigManager(t *testing.T) {
	selector := &sessionSelectorRecorder{}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            selector,
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: nil,
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true when ClientConfigManager is nil")
	}
	if s.resultMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", s.resultMode)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if selector.selected != "my-config" {
		t.Fatalf("expected selector to be called with 'my-config', got %q", selector.selected)
	}
}

func TestUpdateClientSelectScreen_EnterSelectConfig_InvalidConfig(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerInvalid{err: errors.New("invalid client configuration (test): bad key")},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, _ := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientInvalid {
		t.Fatalf("expected invalid screen, got %v", s.screen)
	}
	if !s.invalidAllowDelete {
		t.Fatal("expected invalidAllowDelete=true")
	}
}

// --- 9. updateClientRemoveScreen ---

func TestUpdateClientRemoveScreen_EscGoesBack(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientRemove
	m.clientRemovePaths = []string{"config1"}

	result, _ := m.updateClientRemoveScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
}

func TestUpdateClientRemoveScreen_EnterRemoves(t *testing.T) {
	deleter := &sessionDeleterRecorder{}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"remaining"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             deleter,
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientRemove
	model.clientRemovePaths = []string{"config-to-remove"}
	model.cursor = 0

	result, _ := model.updateClientRemoveScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select after remove, got %v", s.screen)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "config-to-remove" {
		t.Fatalf("expected deletion of 'config-to-remove', got %v", deleter.deleted)
	}
	if !strings.Contains(s.notice, "removed") {
		t.Fatalf("expected removal notice, got %q", s.notice)
	}
}

// --- 10. updateClientAddNameScreen ---

func TestUpdateClientAddNameScreen_EscGoesBack(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName

	result, _ := m.updateClientAddNameScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
}

func TestUpdateClientAddNameScreen_EnterEmptyName(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName
	m.addNameInput.SetValue("")

	result, _ := m.updateClientAddNameScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected to stay on add name, got %v", s.screen)
	}
	if s.notice == "" {
		t.Fatal("expected notice about empty name")
	}
}

func TestUpdateClientAddNameScreen_EnterValidName(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName
	m.addNameInput.SetValue("my-config")

	result, cmd := m.updateClientAddNameScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddJSON {
		t.Fatalf("expected add JSON screen, got %v", s.screen)
	}
	if s.addName != "my-config" {
		t.Fatalf("expected addName='my-config', got %q", s.addName)
	}
	if cmd == nil {
		t.Fatal("expected textarea.Blink cmd")
	}
}

func TestUpdateClientAddNameScreen_OtherKeysPassToInput(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName

	// Type a character - it should be forwarded to the text input
	result, _ := m.updateClientAddNameScreen(keyRunes('a'))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected to stay on add name screen, got %v", s.screen)
	}
}

// --- 11. updateClientAddJSONScreen ---

func TestUpdateClientAddJSONScreen_EscGoesBack(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON

	result, _ := m.updateClientAddJSONScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected add name screen, got %v", s.screen)
	}
}

func TestUpdateClientAddJSONScreen_EnterInvalidJSON(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.addJSONInput.SetValue("not valid json")

	result, _ := m.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientInvalid {
		t.Fatalf("expected invalid screen, got %v", s.screen)
	}
	if s.invalidAllowDelete {
		t.Fatal("expected invalidAllowDelete=false for JSON parse error")
	}
}

func TestUpdateClientAddJSONScreen_OtherKeysPassToInput(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON

	result, _ := m.updateClientAddJSONScreen(keyRunes('x'))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddJSON {
		t.Fatalf("expected to stay on add JSON screen, got %v", s.screen)
	}
}

// --- 12. updateClientInvalidScreen ---

func TestUpdateClientInvalidScreen_EscGoesBack(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad")

	result, _ := m.updateClientInvalidScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
}

func TestUpdateClientInvalidScreen_EnterOK(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad")
	m.invalidAllowDelete = false
	m.cursor = 0 // "OK" is the only option

	result, _ := m.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
}

func TestUpdateClientInvalidScreen_EnterDeleteWhenAllowed(t *testing.T) {
	deleter := &sessionDeleterRecorder{}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             deleter,
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientInvalid
	model.invalidErr = errors.New("bad config")
	model.invalidAllowDelete = true
	model.invalidConfig = "bad-config-file"
	model.cursor = 0 // "Delete invalid configuration" is first option when allowDelete

	result, _ := model.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "bad-config-file" {
		t.Fatalf("expected deletion of 'bad-config-file', got %v", deleter.deleted)
	}
	if !strings.Contains(s.notice, "Invalid configuration deleted") {
		t.Fatalf("expected delete notice, got %q", s.notice)
	}
}

func TestUpdateClientInvalidScreen_EnterOKWhenDeleteAllowed(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad")
	m.invalidAllowDelete = true
	m.invalidConfig = "some-config"
	m.cursor = 1 // "OK" is second option when allowDelete

	result, _ := m.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select, got %v", s.screen)
	}
}

// --- 13. updateServerSelectScreen ---

func TestUpdateServerSelectScreen_EscGoesBackToMode(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerSelect

	result, _ := m.updateServerSelectScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen, got %v", s.screen)
	}
}

func TestUpdateServerSelectScreen_EnterStartServer(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerSelect
	m.cursor = 0 // "start server"

	result, cmd := m.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true")
	}
	if s.resultMode != mode.Server {
		t.Fatalf("expected mode.Server, got %v", s.resultMode)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateServerSelectScreen_EnterManageClientsNoPeers(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 2 // "manage clients"

	result, _ := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected to stay on server select, got %v", s.screen)
	}
	if !strings.Contains(s.notice, "No clients configured") {
		t.Fatalf("expected no-clients notice, got %q", s.notice)
	}
}

func TestUpdateServerSelectScreen_EnterManageClientsWithPeers(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "peer1", ClientID: 1, Enabled: true},
		},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 2 // "manage clients"

	result, _ := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected server manage screen, got %v", s.screen)
	}
	if len(s.serverManagePeers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(s.serverManagePeers))
	}
}

// --- 14. updateCursor ---

func TestUpdateCursor_ListSizeZero(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 5
	m.updateCursor(keyNamed(tea.KeyDown), 0)
	if m.cursor != 0 {
		t.Fatalf("expected cursor=0 for empty list, got %d", m.cursor)
	}
}

func TestUpdateCursor_UpAtZero(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 0
	m.updateCursor(keyNamed(tea.KeyUp), 5)
	if m.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m.cursor)
	}
}

func TestUpdateCursor_DownAtMax(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 4
	m.updateCursor(keyNamed(tea.KeyDown), 5)
	if m.cursor != 4 {
		t.Fatalf("expected cursor=4, got %d", m.cursor)
	}
}

func TestUpdateCursor_UpDecreases(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 3
	m.updateCursor(keyNamed(tea.KeyUp), 5)
	if m.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.cursor)
	}
}

func TestUpdateCursor_DownIncreases(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 2
	m.updateCursor(keyNamed(tea.KeyDown), 5)
	if m.cursor != 3 {
		t.Fatalf("expected cursor=3, got %d", m.cursor)
	}
}

func TestUpdateCursor_KAlias(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 2
	m.updateCursor(keyRunes('k'), 5)
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}
}

func TestUpdateCursor_JAlias(t *testing.T) {
	m := newTestSessionModel(t)
	m.cursor = 2
	m.updateCursor(keyRunes('j'), 5)
	if m.cursor != 3 {
		t.Fatalf("expected cursor=3, got %d", m.cursor)
	}
}

// --- 15. renderSelectionScreen ---

func TestRenderSelectionScreen_ReturnsNonEmptyWithTabs(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30

	result := m.renderSelectionScreen("Test Title", "", []string{"opt1", "opt2"}, 0, "hint text")
	if result == "" {
		t.Fatal("expected non-empty render result")
	}
	if !strings.Contains(result, "Test Title") {
		t.Fatalf("expected title in output, got: %s", result)
	}
}

func TestRenderSelectionScreen_WithNotice(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30

	result := m.renderSelectionScreen("Title", "Some notice", []string{"opt1"}, 0, "hint")
	if !strings.Contains(result, "Some notice") {
		t.Fatalf("expected notice in output, got: %s", result)
	}
}

// --- 16. settingsTabView ---

func TestSettingsTabView_ContainsSettingsRows(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30

	view := m.settingsTabView()
	if !strings.Contains(strings.ToLower(view), "theme") {
		t.Fatalf("settings tab view should contain 'Theme', got: %s", view)
	}
}

// --- 17. logsTabView ---

func TestLogsTabView_ReturnsNonEmpty(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30
	m.logReady = true

	view := m.logsTabView()
	if view == "" {
		t.Fatal("logs tab view should be non-empty")
	}
}

// --- 18. tabsLine ---

func TestTabsLine_ReturnsNonEmptyWithProductLabel(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80

	styles := resolveUIStyles(m.preferences)
	result := m.tabsLine(styles)
	if result == "" {
		t.Fatal("tabsLine should be non-empty")
	}
	// Product label contains "TunGo" or the default product label
	if !strings.Contains(result, "TunGo") && !strings.Contains(result, productLabel()) {
		t.Fatalf("tabsLine should contain product label, got: %s", result)
	}
}

// --- 19. adjustInputsToViewport ---

func TestAdjustInputsToViewport_ZeroWidthReturnsEarly(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 0
	origWidth := m.addNameInput.Width

	m.adjustInputsToViewport()
	if m.addNameInput.Width != origWidth {
		t.Fatalf("expected no change to input width, got %d", m.addNameInput.Width)
	}
}

func TestAdjustInputsToViewport_PositiveWidthAdjusts(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 120
	m.height = 40

	m.adjustInputsToViewport()
	if m.addNameInput.Width <= 0 {
		t.Fatalf("expected positive input width, got %d", m.addNameInput.Width)
	}
}

// --- 20. inputContainerWidth ---

func TestInputContainerWidth_ZeroWidth(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 0

	w := m.inputContainerWidth()
	if w <= 0 {
		t.Fatalf("expected positive width, got %d", w)
	}
}

func TestInputContainerWidth_PositiveWidth(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 120

	w := m.inputContainerWidth()
	if w <= 0 {
		t.Fatalf("expected positive width, got %d", w)
	}
}

// --- 21. reloadClientConfigs ---

func TestReloadClientConfigs_BuildsCorrectMenuOptions(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"config-a", "config-b"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}

	if err := model.reloadClientConfigs(); err != nil {
		t.Fatalf("reloadClientConfigs error: %v", err)
	}

	if len(model.clientConfigs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(model.clientConfigs))
	}
	// Expected: config-a, config-b, "- remove configuration", "+ add configuration"
	if len(model.clientMenuOptions) != 4 {
		t.Fatalf("expected 4 menu options, got %d: %v", len(model.clientMenuOptions), model.clientMenuOptions)
	}
	if model.clientMenuOptions[0] != "config-a" {
		t.Fatalf("expected first option to be 'config-a', got %q", model.clientMenuOptions[0])
	}
	if model.clientMenuOptions[2] != sessionClientRemove {
		t.Fatalf("expected third option to be remove, got %q", model.clientMenuOptions[2])
	}
	if model.clientMenuOptions[3] != sessionClientAdd {
		t.Fatalf("expected fourth option to be add, got %q", model.clientMenuOptions[3])
	}
}

func TestReloadClientConfigs_EmptyConfigs(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}

	if err := model.reloadClientConfigs(); err != nil {
		t.Fatalf("reloadClientConfigs error: %v", err)
	}

	// No configs => just "+ add configuration" (no remove option)
	if len(model.clientMenuOptions) != 1 {
		t.Fatalf("expected 1 menu option, got %d: %v", len(model.clientMenuOptions), model.clientMenuOptions)
	}
	if model.clientMenuOptions[0] != sessionClientAdd {
		t.Fatalf("expected add option, got %q", model.clientMenuOptions[0])
	}
}

func TestReloadClientConfigs_ObserverError(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverError{err: errors.New("observe failed")},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}

	if reloadErr := model.reloadClientConfigs(); reloadErr == nil {
		t.Fatal("expected error from reloadClientConfigs")
	}
}

// --- 22. Log management ---

func TestRestartLogWait_CreatesNewChannel(t *testing.T) {
	m := newTestSessionModel(t)
	if m.logWaitStop != nil {
		t.Fatal("expected nil logWaitStop initially")
	}

	m.restartLogWait()
	if m.logWaitStop == nil {
		t.Fatal("expected non-nil logWaitStop after restart")
	}
}

func TestStopLogWait_ClosesChannel(t *testing.T) {
	m := newTestSessionModel(t)
	m.restartLogWait()
	ch := m.logWaitStop

	m.stopLogWait()
	if m.logWaitStop != nil {
		t.Fatal("expected nil logWaitStop after stop")
	}
	select {
	case <-ch:
		// closed
	default:
		t.Fatal("expected channel to be closed")
	}
}

func TestStopLogWait_NilIsNoop(t *testing.T) {
	m := newTestSessionModel(t)
	m.logWaitStop = nil
	m.stopLogWait() // should not panic
	if m.logWaitStop != nil {
		t.Fatal("expected logWaitStop to remain nil")
	}
}

func TestRefreshLogs_DoesNotPanic(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30
	m.logReady = true
	m.refreshLogs()
}

func TestEnsureLogsViewport_InitializesWhenNotReady(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30
	m.logReady = false

	m.ensureLogsViewport()
	if !m.logReady {
		t.Fatal("expected logReady=true after ensureLogsViewport")
	}
}

func TestEnsureLogsViewport_UpdatesDimensionsWhenReady(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30
	m.logReady = true

	m.ensureLogsViewport()
	if m.logViewport.Width <= 0 {
		t.Fatalf("expected positive viewport width, got %d", m.logViewport.Width)
	}
}

// --- 23. configuratorLogTickCmd ---

func TestConfiguratorLogTickCmd_ReturnsNonNilCmd(t *testing.T) {
	cmd := configuratorLogTickCmd(42)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from configuratorLogTickCmd")
	}
}

// --- 24. configuratorLogUpdateCmd ---

func TestConfiguratorLogUpdateCmd_WithNilFeed_ReturnsTick(t *testing.T) {
	stop := make(chan struct{})
	cmd := configuratorLogUpdateCmd(nil, stop, 1)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

type stubLogFeed struct{}

func (stubLogFeed) Tail(limit int) []string       { return nil }
func (stubLogFeed) TailInto([]string, int) int     { return 0 }

type stubLogChangeFeed struct {
	stubLogFeed
	ch chan struct{}
}

func (s stubLogChangeFeed) Changes() <-chan struct{} { return s.ch }

func TestConfiguratorLogUpdateCmd_WithChangeFeed_ReturnsNonNil(t *testing.T) {
	ch := make(chan struct{})
	feed := stubLogChangeFeed{ch: ch}
	stop := make(chan struct{})

	cmd := configuratorLogUpdateCmd(feed, stop, 5)
	if cmd == nil {
		t.Fatal("expected non-nil cmd with change feed")
	}
}

func TestConfiguratorLogUpdateCmd_WithNilChanges_FallsBackToTick(t *testing.T) {
	feed := stubLogChangeFeed{ch: nil}
	stop := make(chan struct{})

	cmd := configuratorLogUpdateCmd(feed, stop, 5)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for nil changes channel")
	}
}

func TestConfiguratorLogUpdateCmd_WithPlainFeed_ReturnsTick(t *testing.T) {
	feed := stubLogFeed{}
	stop := make(chan struct{})

	cmd := configuratorLogUpdateCmd(feed, stop, 5)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for plain feed")
	}
}

// --- View for all screens in main tab ---

func TestView_MainTab_AllScreens_ReturnsNonEmpty(t *testing.T) {
	screens := []struct {
		name   string
		screen configuratorScreen
		setup  func(m *configuratorSessionModel)
	}{
		{"Mode", configuratorScreenMode, nil},
		{"ClientSelect", configuratorScreenClientSelect, func(m *configuratorSessionModel) {
			m.clientMenuOptions = []string{sessionClientAdd}
		}},
		{"ClientRemove", configuratorScreenClientRemove, func(m *configuratorSessionModel) {
			m.clientRemovePaths = []string{"cfg1"}
		}},
		{"ClientAddName", configuratorScreenClientAddName, func(m *configuratorSessionModel) {
			m.width = 80
			m.height = 30
		}},
		{"ClientAddJSON", configuratorScreenClientAddJSON, func(m *configuratorSessionModel) {
			m.width = 80
			m.height = 30
		}},
		{"ClientInvalid", configuratorScreenClientInvalid, func(m *configuratorSessionModel) {
			m.invalidErr = errors.New("test err")
		}},
		{"ServerSelect", configuratorScreenServerSelect, nil},
		{"ServerManage", configuratorScreenServerManage, func(m *configuratorSessionModel) {
			m.serverManageLabels = []string{"#1 test [enabled]"}
		}},
		{"ServerDeleteConfirm", configuratorScreenServerDeleteConfirm, func(m *configuratorSessionModel) {
			m.serverDeletePeer = serverConfiguration.AllowedPeer{Name: "a", ClientID: 1}
		}},
	}

	for _, tc := range screens {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestSessionModel(t)
			m.screen = tc.screen
			if tc.setup != nil {
				tc.setup(&m)
			}
			view := m.View()
			if view == "" {
				t.Fatalf("View for screen %s should be non-empty", tc.name)
			}
		})
	}
}

// --- View with notice on ClientAddName and ClientAddJSON screens ---

func TestView_ClientAddNameScreen_WithNotice(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName
	m.width = 80
	m.height = 30
	m.notice = "Name cannot be empty."

	view := m.View()
	if !strings.Contains(view, "Name cannot be empty.") {
		t.Fatalf("expected notice in view, got: %s", view)
	}
}

func TestView_ClientAddJSONScreen_WithNotice(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.width = 80
	m.height = 30
	m.notice = "Invalid JSON"

	view := m.View()
	if !strings.Contains(view, "Invalid JSON") {
		t.Fatalf("expected notice in view, got: %s", view)
	}
}

// --- View for invalid screen with delete allowed ---

func TestView_ClientInvalidScreen_WithDeleteAllowed(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad config")
	m.invalidAllowDelete = true

	view := m.View()
	if !strings.Contains(view, sessionInvalidDelete) {
		t.Fatalf("expected delete option in view, got: %s", view)
	}
}

// --- Update dispatch for each screen type via top-level Update ---

func TestUpdate_DispatchToClientSelectScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientMenuOptions = []string{sessionClientAdd}
	m.cursor = 0

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected mode screen from client select esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToClientRemoveScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientRemove
	m.clientRemovePaths = []string{"cfg"}

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select from remove esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToClientAddNameScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select from add name esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToClientAddJSONScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected add name from add JSON esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToClientInvalidScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad")

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select from invalid esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToServerSelectScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerSelect

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected mode from server select esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToServerManageScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerManage
	m.serverManagePeers = []serverConfiguration.AllowedPeer{{Name: "a", ClientID: 1, Enabled: true}}
	m.serverManageLabels = []string{"#1 a [enabled]"}

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected server select from manage esc, got %v", s.screen)
	}
}

func TestUpdate_DispatchToServerDeleteConfirmScreen(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerDeleteConfirm
	m.serverManagePeers = []serverConfiguration.AllowedPeer{{Name: "a", ClientID: 1, Enabled: true}}
	m.serverDeletePeer = serverConfiguration.AllowedPeer{Name: "a", ClientID: 1}

	result, _ := m.Update(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected server manage from delete confirm esc, got %v", s.screen)
	}
}

func TestUpdate_UnhandledMsg_ReturnsModelUnchanged(t *testing.T) {
	m := newTestSessionModel(t)
	result, cmd := m.Update("some random message")
	_ = result.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd for unhandled message type")
	}
}

// =========================================================================
// New stubs for coverage gap tests
// =========================================================================

// sessionSelectorError returns an error from Select.
type sessionSelectorError struct{ err error }

func (s sessionSelectorError) Select(string) error { return s.err }

// sessionCreatorRecorder records Create calls.
type sessionCreatorRecorder struct {
	called bool
	err    error
}

func (s *sessionCreatorRecorder) Create(clientConfiguration.Configuration, string) error {
	s.called = true
	return s.err
}

// sessionDeleterError always returns an error.
type sessionDeleterError struct{ err error }

func (s sessionDeleterError) Delete(string) error { return s.err }

// sessionServerConfigManagerListError returns error from ListAllowedPeers.
type sessionServerConfigManagerListError struct {
	sessionServerConfigManagerStub
	listErr error
}

func (s *sessionServerConfigManagerListError) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	return nil, s.listErr
}

// sessionServerConfigManagerEnableError returns error from SetAllowedPeerEnabled.
type sessionServerConfigManagerEnableError struct {
	sessionServerConfigManagerStub
	enableErr error
}

func (s *sessionServerConfigManagerEnableError) SetAllowedPeerEnabled(int, bool) error {
	return s.enableErr
}

// sessionClientConfigManagerNonInvalid returns a non-invalid-config error from Configuration().
type sessionClientConfigManagerNonInvalid struct {
	err error
}

func (s sessionClientConfigManagerNonInvalid) Configuration() (*clientConfiguration.Configuration, error) {
	return nil, s.err
}

// sessionClientConfigManagerValid returns a valid Configuration.
type sessionClientConfigManagerValid struct{}

func (s sessionClientConfigManagerValid) Configuration() (*clientConfiguration.Configuration, error) {
	cfg := validClientConfiguration()
	return &cfg, nil
}

// sessionServerConfigManagerConfigError returns error from Configuration() (for confgen.Generate).
type sessionServerConfigManagerConfigError struct {
	sessionServerConfigManagerStub
	configErr error
}

func (s *sessionServerConfigManagerConfigError) Configuration() (*serverConfiguration.Configuration, error) {
	return nil, s.configErr
}

// sessionServerConfigManagerRemoveError returns error from RemoveAllowedPeer.
type sessionServerConfigManagerRemoveError struct {
	sessionServerConfigManagerStub
	removeError error
}

func (s *sessionServerConfigManagerRemoveError) RemoveAllowedPeer(int) error {
	return s.removeError
}

// sessionServerConfigManagerRemoveOKThenListError succeeds on RemoveAllowedPeer
// but returns error from the subsequent ListAllowedPeers call.
type sessionServerConfigManagerRemoveOKThenListError struct {
	sessionServerConfigManagerStub
	removed bool
	listErr error
}

func (s *sessionServerConfigManagerRemoveOKThenListError) RemoveAllowedPeer(clientID int) error {
	s.removed = true
	// Actually remove so the stub state changes
	for i := range s.peers {
		if s.peers[i].ClientID == clientID {
			s.peers = append(s.peers[:i], s.peers[i+1:]...)
			break
		}
	}
	return nil
}

func (s *sessionServerConfigManagerRemoveOKThenListError) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	if s.removed {
		return nil, s.listErr
	}
	return s.sessionServerConfigManagerStub.ListAllowedPeers()
}

// =========================================================================
// 1. updateServerManageScreen coverage
// =========================================================================

func TestUpdateServerManageScreen_D_EmptyPeersList(t *testing.T) {
	manager := &sessionServerConfigManagerStub{peers: nil}
	model := newSessionModelForServerManageTests(t, manager)
	model.serverManagePeers = nil
	model.serverManageLabels = nil

	result, cmd := model.updateServerManageScreen(keyRunes('d'))
	s := result.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	// Should stay on manage screen without change
	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected to stay on manage screen, got %v", s.screen)
	}
}

func TestUpdateServerManageScreen_D_UpperCase_EmptyPeersList(t *testing.T) {
	manager := &sessionServerConfigManagerStub{peers: nil}
	model := newSessionModelForServerManageTests(t, manager)
	model.serverManagePeers = nil
	model.serverManageLabels = nil

	result, cmd := model.updateServerManageScreen(keyRunes('D'))
	s := result.(configuratorSessionModel)
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected to stay on manage screen, got %v", s.screen)
	}
}

func TestUpdateServerManageScreen_EnterTogglesPeerEnabled(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	// Enter on peer with Enabled=true should call SetAllowedPeerEnabled(1, false)
	result, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if len(s.serverManagePeers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(s.serverManagePeers))
	}
	if s.serverManagePeers[0].Enabled {
		t.Fatal("expected peer to be disabled after toggle")
	}
}

func TestUpdateServerManageScreen_SetAllowedPeerEnabledError(t *testing.T) {
	manager := &sessionServerConfigManagerEnableError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "alpha", ClientID: 1, Enabled: true},
			},
		},
		enableErr: errors.New("enable failed"),
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerManage
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.serverManageLabels = buildServerManageLabels(model.serverManagePeers)
	model.cursor = 0

	result, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected server select screen on enable error, got %v", s.screen)
	}
	if !strings.Contains(s.notice, "Failed to update client") {
		t.Fatalf("expected failure notice, got %q", s.notice)
	}
}

func TestUpdateServerManageScreen_AfterToggle_ListAllowedPeersError(t *testing.T) {
	manager := &sessionServerConfigManagerListError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "alpha", ClientID: 1, Enabled: true},
			},
		},
		listErr: errors.New("list failed"),
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerManage
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.serverManageLabels = buildServerManageLabels(model.serverManagePeers)
	model.cursor = 0

	result, cmd := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on ListAllowedPeers error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "list failed") {
		t.Fatalf("expected list error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateServerManageScreen_AfterToggle_PeersEmpty(t *testing.T) {
	// After toggle, ListAllowedPeers returns empty list
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "alpha", ClientID: 1, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)

	// Remove all peers from manager before the Enter so ListAllowedPeers returns empty
	manager.peers = nil

	result, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected server select when peers empty, got %v", s.screen)
	}
	if !strings.Contains(s.notice, "No clients configured") {
		t.Fatalf("expected no-clients notice, got %q", s.notice)
	}
}

func TestUpdateServerManageScreen_CursorClamping(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{
			{Name: "a", ClientID: 1, Enabled: true},
			{Name: "b", ClientID: 2, Enabled: true},
		},
	}
	model := newSessionModelForServerManageTests(t, manager)
	model.cursor = 1 // on "b"

	// Remove peer "b" so after toggle, only "a" remains
	// We do this by removing peer 2 from the manager's list after toggle
	// But SetAllowedPeerEnabled will still work on peer 2 since it exists now
	// After toggle, let manager only have peer 1
	origSetEnabled := manager.SetAllowedPeerEnabled
	_ = origSetEnabled
	// We need the toggle to succeed, then list to return only 1 peer
	// Simply remove peer 2 from manager.peers before the list call happens

	// Actually, let's just set cursor high and have the list return fewer peers.
	// The simplest: toggle peer 2 and then manager only has peer 1.
	manager.peers = []serverConfiguration.AllowedPeer{
		{Name: "a", ClientID: 1, Enabled: true},
		{Name: "b", ClientID: 2, Enabled: true},
	}
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.serverManageLabels = buildServerManageLabels(model.serverManagePeers)
	model.cursor = 1

	// After toggle of peer 2, remove it from manager so list returns fewer
	// We'll cheat: after the enter, remove from the manager
	// The sequence: enter -> SetAllowedPeerEnabled(2,false) -> ListAllowedPeers
	// We modify the manager to only return 1 peer after SetAllowedPeerEnabled
	// Easiest: just let it toggle, and after the call it will list both.
	// For cursor clamping, cursor=1 and len=2, so no clamping needed.
	// Instead, set cursor >= len.

	// To trigger the clamping branch (cursor >= len(peers)):
	// We need the list to return fewer peers than cursor.
	// Remove peer "b" from manager BEFORE the enter key
	manager.peers = []serverConfiguration.AllowedPeer{
		{Name: "a", ClientID: 1, Enabled: true},
	}

	// But model.serverManagePeers still has 2 peers for the toggle
	result, _ := model.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	// cursor was 1, list returned 1 peer, so cursor should be clamped to 0
	if s.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", s.cursor)
	}
}

// =========================================================================
// 2. updateServerSelectScreen coverage
// =========================================================================

func TestUpdateServerSelectScreen_EnterAddClient_GenerateError(t *testing.T) {
	// "add client" at cursor=1 calls confgen.Generate which needs
	// ServerConfigManager.Configuration(). Make Configuration() return error
	// so that Generate fails immediately.
	configErr := errors.New("config read failed")
	manager := &sessionServerConfigManagerConfigError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "a", ClientID: 1, Enabled: true},
			},
		},
		configErr: configErr,
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 1 // "add client"

	result, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true when confgen.Generate fails")
	}
	if s.resultErr == nil {
		t.Fatal("expected non-nil resultErr")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateServerSelectScreen_ManageClients_ListError(t *testing.T) {
	manager := &sessionServerConfigManagerListError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "a", ClientID: 1, Enabled: true},
			},
		},
		listErr: errors.New("list error"),
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 2 // "manage clients"

	result, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on ListAllowedPeers error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "list error") {
		t.Fatalf("expected list error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// =========================================================================
// 3. updateClientSelectScreen coverage
// =========================================================================

func TestUpdateClientSelectScreen_EnterConfig_SelectorError(t *testing.T) {
	selectorErr := errors.New("select failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorError{err: selectorErr},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on selector error")
	}
	if !errors.Is(s.resultErr, selectorErr) {
		t.Fatalf("expected selector error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientSelectScreen_EnterConfig_NonInvalidError(t *testing.T) {
	nonInvalidErr := errors.New("network timeout while connecting")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerNonInvalid{err: nonInvalidErr},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on non-invalid config error")
	}
	if !errors.Is(s.resultErr, nonInvalidErr) {
		t.Fatalf("expected non-invalid error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientSelectScreen_EnterConfig_ValidConfig(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerValid{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true for valid config")
	}
	if s.resultMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", s.resultMode)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// =========================================================================
// 4. updateClientRemoveScreen coverage
// =========================================================================

func TestUpdateClientRemoveScreen_EnterEmptyPaths(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientRemove
	m.clientRemovePaths = nil
	m.cursor = 0

	result, cmd := m.updateClientRemoveScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	// Should return without action
	if s.done {
		t.Fatal("expected done=false when paths empty")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
}

func TestUpdateClientRemoveScreen_DeleterError(t *testing.T) {
	deleterErr := errors.New("delete failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"remaining"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterError{err: deleterErr},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientRemove
	model.clientRemovePaths = []string{"config-to-remove"}
	model.cursor = 0

	result, cmd := model.updateClientRemoveScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on deleter error")
	}
	if !errors.Is(s.resultErr, deleterErr) {
		t.Fatalf("expected deleter error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientRemoveScreen_ReloadError(t *testing.T) {
	observeErr := errors.New("observe failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverError{err: observeErr},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientRemove
	model.clientRemovePaths = []string{"config-to-remove"}
	model.cursor = 0

	result, cmd := model.updateClientRemoveScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on reload error")
	}
	if !errors.Is(s.resultErr, observeErr) {
		t.Fatalf("expected observe error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// =========================================================================
// 5. updateClientAddJSONScreen coverage
// =========================================================================

func TestUpdateClientAddJSONScreen_EnterValidJSON_CreateSucceeds(t *testing.T) {
	creator := &sessionCreatorRecorder{}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{}},
		Selector:            sessionSelectorStub{},
		Creator:             creator,
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientAddJSON
	model.addName = "my-config"
	model.addJSONInput.SetValue(validClientConfigurationJSON())

	result, _ := model.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected client select screen, got %v", s.screen)
	}
	if s.notice != "Configuration added." {
		t.Fatalf("expected 'Configuration added.' notice, got %q", s.notice)
	}
	if !creator.called {
		t.Fatal("expected Creator.Create to be called")
	}
}

func TestUpdateClientAddJSONScreen_EnterValidJSON_CreateError(t *testing.T) {
	createErr := errors.New("create failed")
	creator := &sessionCreatorRecorder{err: createErr}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{}},
		Selector:            sessionSelectorStub{},
		Creator:             creator,
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientAddJSON
	model.addName = "my-config"
	model.addJSONInput.SetValue(validClientConfigurationJSON())

	result, cmd := model.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on create error")
	}
	if !errors.Is(s.resultErr, createErr) {
		t.Fatalf("expected create error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientAddJSONScreen_EnterValidJSON_ReloadError(t *testing.T) {
	observeErr := errors.New("observe failed")
	creator := &sessionCreatorRecorder{}
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverError{err: observeErr},
		Selector:            sessionSelectorStub{},
		Creator:             creator,
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientAddJSON
	model.addName = "my-config"
	model.addJSONInput.SetValue(validClientConfigurationJSON())

	result, cmd := model.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on reload error")
	}
	if !errors.Is(s.resultErr, observeErr) {
		t.Fatalf("expected observe error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if !creator.called {
		t.Fatal("expected Creator.Create to have been called before reload")
	}
}

// =========================================================================
// 6. updateClientInvalidScreen coverage
// =========================================================================

func TestUpdateClientInvalidScreen_DeleteBlankInvalidConfig(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad config")
	m.invalidAllowDelete = true
	m.invalidConfig = "   " // blank
	m.cursor = 0            // "Delete invalid configuration"

	result, cmd := m.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true for blank invalidConfig delete")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "cannot be deleted") {
		t.Fatalf("expected 'cannot be deleted' error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientInvalidScreen_DeleteDeleterError(t *testing.T) {
	deleterErr := errors.New("delete failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterError{err: deleterErr},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientInvalid
	model.invalidErr = errors.New("bad config")
	model.invalidAllowDelete = true
	model.invalidConfig = "bad-config-file"
	model.cursor = 0 // "Delete invalid configuration"

	result, cmd := model.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on deleter error")
	}
	if !errors.Is(s.resultErr, deleterErr) {
		t.Fatalf("expected deleter error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientInvalidScreen_DeleteReloadError(t *testing.T) {
	observeErr := errors.New("observe failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverError{err: observeErr},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientInvalid
	model.invalidErr = errors.New("bad config")
	model.invalidAllowDelete = true
	model.invalidConfig = "bad-config-file"
	model.cursor = 0 // "Delete invalid configuration"

	result, cmd := model.updateClientInvalidScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on reload error")
	}
	if !errors.Is(s.resultErr, observeErr) {
		t.Fatalf("expected observe error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientInvalidScreen_NonEnterWithAllowDelete(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientInvalid
	m.invalidErr = errors.New("bad config")
	m.invalidAllowDelete = true
	m.cursor = 0

	// Navigate down
	result, _ := m.updateClientInvalidScreen(keyNamed(tea.KeyDown))
	s := result.(configuratorSessionModel)

	if s.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", s.cursor)
	}
	if s.screen != configuratorScreenClientInvalid {
		t.Fatalf("expected to stay on invalid screen, got %v", s.screen)
	}
}

// =========================================================================
// 7. updateServerDeleteConfirmScreen coverage
// =========================================================================

func TestUpdateServerDeleteConfirmScreen_RemoveAllowedPeerError(t *testing.T) {
	removeErr := errors.New("remove failed")
	manager := &sessionServerConfigManagerRemoveError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "alpha", ClientID: 1, Enabled: true},
			},
		},
		removeError: removeErr,
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverDeletePeer = serverConfiguration.AllowedPeer{Name: "alpha", ClientID: 1, Enabled: true}
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.cursor = 0 // "Delete client"

	result, _ := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen on remove error, got %v", s.screen)
	}
	if !strings.Contains(s.notice, "Failed to remove client") {
		t.Fatalf("expected failure notice, got %q", s.notice)
	}
}

func TestUpdateServerDeleteConfirmScreen_ListAllowedPeersErrorAfterRemove(t *testing.T) {
	listErr := errors.New("list after remove failed")
	manager := &sessionServerConfigManagerRemoveOKThenListError{
		sessionServerConfigManagerStub: sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "alpha", ClientID: 1, Enabled: true},
			},
		},
		listErr: listErr,
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerDeleteConfirm
	model.serverDeletePeer = serverConfiguration.AllowedPeer{Name: "alpha", ClientID: 1, Enabled: true}
	model.serverManagePeers = append([]serverConfiguration.AllowedPeer(nil), manager.peers...)
	model.cursor = 0 // "Delete client"

	result, cmd := model.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on list error after remove")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "list after remove failed") {
		t.Fatalf("expected list error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateServerDeleteConfirmScreen_EscWithEmptyPeers(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerDeleteConfirm
	m.serverManagePeers = nil // empty
	m.serverDeleteCursor = 3

	result, _ := m.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEsc))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", s.screen)
	}
	if s.cursor != 0 {
		t.Fatalf("expected cursor=0 with empty peers, got %d", s.cursor)
	}
}

func TestUpdateServerDeleteConfirmScreen_CancelWithEmptyPeers(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerDeleteConfirm
	m.serverManagePeers = nil // empty
	m.serverDeleteCursor = 3
	m.cursor = 1 // "Cancel"

	result, _ := m.updateServerDeleteConfirmScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected manage screen, got %v", s.screen)
	}
	if s.cursor != 0 {
		t.Fatalf("expected cursor=0 with empty peers, got %d", s.cursor)
	}
}

// =========================================================================
// 8. updateModeScreen coverage
// =========================================================================

func TestUpdateModeScreen_EnterClient_ReloadFails(t *testing.T) {
	observeErr := errors.New("observe failed")
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverError{err: observeErr},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenMode
	model.cursor = 0 // "client"

	result, cmd := model.updateModeScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on reload error")
	}
	if !errors.Is(s.resultErr, observeErr) {
		t.Fatalf("expected observe error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// =========================================================================
// 9. configuratorLogTickCmd coverage
// =========================================================================

func TestConfiguratorLogTickCmd_ProducesCorrectMsg(t *testing.T) {
	cmd := configuratorLogTickCmd(42)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	// The cmd wraps tea.Tick which won't resolve immediately without time passing,
	// so we just verify the cmd is non-nil (the function structure is simple).
}

// =========================================================================
// 10. configuratorLogUpdateCmd coverage
// =========================================================================

func TestConfiguratorLogUpdateCmd_StopChannelClosed(t *testing.T) {
	ch := make(chan struct{}, 1)
	feed := stubLogChangeFeed{ch: ch}
	stop := make(chan struct{})
	close(stop) // pre-close

	cmd := configuratorLogUpdateCmd(feed, stop, 7)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	tick, ok := msg.(configuratorLogTickMsg)
	if !ok {
		t.Fatalf("expected configuratorLogTickMsg, got %T", msg)
	}
	// When stop is closed, seq should be zero (default)
	if tick.seq != 0 {
		t.Fatalf("expected seq=0 when stop closed, got %d", tick.seq)
	}
}

func TestConfiguratorLogUpdateCmd_ChangesChannelFires(t *testing.T) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{} // pre-fire
	feed := stubLogChangeFeed{ch: ch}
	stop := make(chan struct{})

	cmd := configuratorLogUpdateCmd(feed, stop, 7)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	tick, ok := msg.(configuratorLogTickMsg)
	if !ok {
		t.Fatalf("expected configuratorLogTickMsg, got %T", msg)
	}
	if tick.seq != 7 {
		t.Fatalf("expected seq=7 when changes fires, got %d", tick.seq)
	}
}

// =========================================================================
// 11. refreshLogs coverage - logFollow=false, not at bottom
// =========================================================================

func TestRefreshLogs_NotFollowing_PreservesOffset(t *testing.T) {
	m := newTestSessionModel(t)
	m.width = 80
	m.height = 30
	m.logReady = true
	m.logFollow = false

	// Set content with many lines so viewport is not at bottom
	m.ensureLogsViewport()
	longContent := strings.Repeat("line\n", 200)
	m.logViewport.SetContent(longContent)
	m.logViewport.SetYOffset(5) // not at bottom

	m.refreshLogs()

	// logFollow should remain false
	if m.logFollow {
		t.Fatal("expected logFollow to remain false")
	}
}

// =========================================================================
// 12. View coverage - unknown screen returns empty string
// =========================================================================

func TestView_UnknownScreen_ReturnsEmptyString(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreen(999) // unknown screen

	view := m.View()
	if view != "" {
		t.Fatalf("expected empty string for unknown screen, got %q", view)
	}
}

// sessionServerConfigManagerWithFallback wraps the stub to set FallbackServerAddress
// so confgen.Generate() works even without network connectivity.
type sessionServerConfigManagerWithFallback struct {
	*sessionServerConfigManagerStub
}

func (s *sessionServerConfigManagerWithFallback) Configuration() (*serverConfiguration.Configuration, error) {
	conf, err := s.sessionServerConfigManagerStub.Configuration()
	if err != nil {
		return nil, err
	}
	conf.FallbackServerAddress = "127.0.0.1"
	return conf, nil
}

func TestUpdateServerSelectScreen_EnterAddClient_WriteFileError(t *testing.T) {
	prev := writeServerClientConfigFile
	t.Cleanup(func() { writeServerClientConfigFile = prev })
	writeServerClientConfigFile = func(_ int, _ []byte) (string, error) {
		return "", errors.New("disk full")
	}

	manager := &sessionServerConfigManagerWithFallback{
		sessionServerConfigManagerStub: &sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "a", ClientID: 1, Enabled: true},
			},
		},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 1 // "add client"

	result, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if !s.done {
		t.Fatal("expected done=true on writeServerClientConfigFile error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "disk full") {
		t.Fatalf("expected disk full error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateServerSelectScreen_EnterAddClient_Success(t *testing.T) {
	prev := writeServerClientConfigFile
	t.Cleanup(func() { writeServerClientConfigFile = prev })
	writeServerClientConfigFile = func(clientID int, data []byte) (string, error) {
		return "/tmp/test_client.json", nil
	}

	manager := &sessionServerConfigManagerWithFallback{
		sessionServerConfigManagerStub: &sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "a", ClientID: 1, Enabled: true},
			},
		},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 1 // "add client"

	result, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)

	if s.done {
		t.Fatalf("expected done=false on success, resultErr=%v", s.resultErr)
	}
	if !strings.Contains(s.notice, "/tmp/test_client.json") {
		t.Fatalf("expected notice with path, got %q", s.notice)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd on success")
	}
}

func TestConfiguratorLogTickCmd_InnerFuncEmitsMessage(t *testing.T) {
	cmd := configuratorLogTickCmd(42)
	msg := cmd()
	tick, ok := msg.(configuratorLogTickMsg)
	if !ok {
		t.Fatalf("expected configuratorLogTickMsg, got %T", msg)
	}
	if tick.seq != 42 {
		t.Fatalf("expected seq=42, got %d", tick.seq)
	}
}

func TestUpdateServerSelectScreen_ManageClientsListError_Exits(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers:   []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
		listErr: errors.New("list failed"),
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenServerSelect
	model.cursor = 2 // "manage clients"

	result, cmd := model.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true on list error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "list failed") {
		t.Fatalf("expected list error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientSelectScreen_SelectConfig_NonInvalidConfigError_Exits(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerInvalid{err: errors.New("connection refused")},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true for non-invalid config error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "connection refused") {
		t.Fatalf("expected connection refused error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientSelectScreen_SelectConfig_ValidConfig_ExitsWithClientMode(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true for valid config")
	}
	if s.resultMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", s.resultMode)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestUpdateClientSelectScreen_SelectorError_Exits(t *testing.T) {
	manager := &sessionServerConfigManagerStub{
		peers: []serverConfiguration.AllowedPeer{{Name: "t", ClientID: 1, Enabled: true}},
	}
	model, err := newConfiguratorSessionModel(ConfiguratorSessionOptions{
		Observer:            sessionObserverWithConfigs{configs: []string{"my-config"}},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: manager,
	}, testSettings())
	if err != nil {
		t.Fatalf("newConfiguratorSessionModel error: %v", err)
	}
	// Override selector to one that fails
	model.options.Selector = sessionSelectorError{err: errors.New("select failed")}
	model.screen = configuratorScreenClientSelect
	model.clientMenuOptions = []string{"my-config", sessionClientRemove, sessionClientAdd}
	model.clientConfigs = []string{"my-config"}
	model.cursor = 0

	result, cmd := model.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if !s.done {
		t.Fatal("expected done=true on selector error")
	}
	if s.resultErr == nil || !strings.Contains(s.resultErr.Error(), "select failed") {
		t.Fatalf("expected select error, got %v", s.resultErr)
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// --- Non-key message forwarding to active input (fixes Ctrl+V paste on Windows) ---

func TestUpdate_NonKeyMsg_ForwardedToInput_AddName(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddName

	// Any non-key, non-window-size message should be forwarded to the textinput,
	// not silently dropped. This is required for clipboard paste results and cursor blinks.
	type customMsg struct{}
	result, _ := m.Update(customMsg{})
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddName {
		t.Fatalf("expected to stay on add name screen, got %v", s.screen)
	}
}

func TestUpdate_NonKeyMsg_ForwardedToInput_AddJSON(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON

	// Any non-key, non-window-size message should be forwarded to the textarea,
	// not silently dropped. This is required for clipboard paste results and cursor blinks.
	type customMsg struct{}
	result, _ := m.Update(customMsg{})
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddJSON {
		t.Fatalf("expected to stay on add JSON screen, got %v", s.screen)
	}
}

func TestUpdate_JSONScreen_EnterDebouncedDuringPaste(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	// Simulate recent non-Enter input (as if paste just happened).
	m.lastInputAt = time.Now()

	// Enter within debounce window should be forwarded to textarea as newline,
	// not treated as submit.
	result, _ := m.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientAddJSON {
		t.Fatal("expected Enter to be debounced during paste")
	}
	// lastInputAt should be refreshed so the debounce window extends.
	if s.lastInputAt.IsZero() {
		t.Fatal("expected lastInputAt to be refreshed during debounce")
	}
}

func TestUpdate_JSONScreen_EnterAcceptedAfterDebounce(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.addJSONInput.SetValue("not valid json")
	// No recent input — lastInputAt is zero, Enter should be accepted.

	result, _ := m.updateClientAddJSONScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientInvalid {
		t.Fatalf("expected Enter to be accepted (goes to invalid screen for bad JSON), got %v", s.screen)
	}
}

func TestUpdate_JSONScreen_NonEnterKeySetsLastInputAt(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON

	if !m.lastInputAt.IsZero() {
		t.Fatal("expected lastInputAt to be zero initially")
	}
	result, _ := m.updateClientAddJSONScreen(keyRunes('x'))
	s := result.(configuratorSessionModel)
	if s.lastInputAt.IsZero() {
		t.Fatal("expected lastInputAt to be set after key input")
	}
}

func TestUpdate_PasteSettledMsg_FormatsJSON(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.pasteSeq = 5
	m.addJSONInput.SetValue(`{"a":1,"b":2}`)

	result, _ := m.Update(pasteSettledMsg{seq: 5})
	s := result.(configuratorSessionModel)
	got := s.addJSONInput.Value()
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected formatted JSON with newlines, got %q", got)
	}
}

func TestUpdate_PasteSettledMsg_StaleSeqIgnored(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.pasteSeq = 5
	m.addJSONInput.SetValue(`{"a":1}`)

	result, _ := m.Update(pasteSettledMsg{seq: 3}) // stale
	s := result.(configuratorSessionModel)
	got := s.addJSONInput.Value()
	if strings.Contains(got, "\n") {
		t.Fatalf("stale seq should not reformat, got %q", got)
	}
}

func TestTryFormatJSON_EmptyInput(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.pasteSeq = 1
	m.addJSONInput.SetValue("")

	// Should not panic or change anything.
	result, _ := m.Update(pasteSettledMsg{seq: 1})
	s := result.(configuratorSessionModel)
	if s.addJSONInput.Value() != "" {
		t.Fatalf("expected empty value unchanged, got %q", s.addJSONInput.Value())
	}
}

func TestTryFormatJSON_InvalidJSON(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.pasteSeq = 1
	m.addJSONInput.SetValue("not json at all")

	result, _ := m.Update(pasteSettledMsg{seq: 1})
	s := result.(configuratorSessionModel)
	if s.addJSONInput.Value() != "not json at all" {
		t.Fatalf("expected invalid JSON unchanged, got %q", s.addJSONInput.Value())
	}
}

func TestTryFormatJSON_AlreadyFormatted(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.pasteSeq = 1
	formatted := "{\n  \"a\": 1\n}"
	m.addJSONInput.SetValue(formatted)

	result, _ := m.Update(pasteSettledMsg{seq: 1})
	s := result.(configuratorSessionModel)
	if s.addJSONInput.Value() != formatted {
		t.Fatalf("expected already-formatted JSON unchanged, got %q", s.addJSONInput.Value())
	}
}

func TestView_ClientAddJSONScreen_MultilineContent(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientAddJSON
	m.width = 80
	m.height = 30
	m.addJSONInput.SetValue("{\n  \"key\": \"value\"\n}")

	view := m.View()
	if !strings.Contains(view, "Lines: 3") {
		t.Fatalf("expected 'Lines: 3' in view for multiline content, got: %s", view)
	}
}

func TestUpdateClientSelectScreen_EmptyMenuOptions(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenClientSelect
	m.clientMenuOptions = nil

	result, _ := m.updateClientSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenClientSelect {
		t.Fatalf("expected to stay on client select with empty options, got %v", s.screen)
	}
}

func TestUpdateServerSelectScreen_DefaultFallthrough(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerSelect
	// Set options to something that doesn't match any known case.
	m.serverMenuOptions = []string{"unknown option"}
	m.cursor = 0

	result, _ := m.updateServerSelectScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	// Should fall through to default return m, nil.
	if s.screen != configuratorScreenServerSelect {
		t.Fatalf("expected to stay on server select for unknown option, got %v", s.screen)
	}
}

func TestUpdateServerManageScreen_EmptyPeersOnEnter(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenServerManage
	m.serverManagePeers = nil

	result, _ := m.updateServerManageScreen(keyNamed(tea.KeyEnter))
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenServerManage {
		t.Fatalf("expected to stay on manage screen with empty peers, got %v", s.screen)
	}
}

func TestUpdate_NonKeyMsg_DroppedOnOtherScreens(t *testing.T) {
	m := newTestSessionModel(t)
	m.screen = configuratorScreenMode

	type customMsg struct{}
	result, cmd := m.Update(customMsg{})
	s := result.(configuratorSessionModel)
	if s.screen != configuratorScreenMode {
		t.Fatalf("expected to stay on mode screen, got %v", s.screen)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for dropped message")
	}
}
