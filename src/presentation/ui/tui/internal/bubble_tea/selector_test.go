package bubble_tea

import (
	"fmt"
	"strings"
	"testing"

	"tungo/presentation/ui/tui/internal/ui/value_objects"

	tea "charm.land/bubbletea/v2"
)

type mockColorizer struct {
	calls int
	lastS string
}

func (m *mockColorizer) ColorizeString(
	s string,
	_, _ value_objects.Color,
) string {
	m.calls++
	m.lastS = s
	return "[[" + s + "]]"
}

func newTestSelector(options ...string) (Selector, *mockColorizer) {
	col := &mockColorizer{}
	return NewSelector(
		"Select option:",
		options,
		col,
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	), col
}

func TestNewSelector(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	if sel.placeholder != "Select option:" {
		t.Errorf("expected placeholder %q, got %q", "Select option:", sel.placeholder)
	}
	if len(sel.options) != 2 {
		t.Errorf("expected 2 options, got %d", len(sel.options))
	}
}

func TestSelector_Init(t *testing.T) {
	sel, _ := newTestSelector("a")
	if cmd := sel.Init(); cmd != nil {
		t.Errorf("expected Init to return nil and start log tick only on Logs tab")
	}
}

func TestSelector_UpdateUp(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	sel.cursor = 1
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector")
	}
	if updatedSel.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateUp_AtTop_NoChange(t *testing.T) {
	sel, _ := newTestSelector("a", "b")
	sel.cursor = 0
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateDown(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateDown_AtBottom_NoChange(t *testing.T) {
	sel, _ := newTestSelector("a", "b")
	sel.cursor = 1
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateEnter_FirstTime_SetsChoice_Quits(t *testing.T) {
	sel, _ := newTestSelector("client", "server")
	sel.cursor = 0
	updatedModel, cmd := sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updatedSel := updatedModel.(Selector)

	if updatedSel.choice != "client" {
		t.Errorf("expected choice 'client', got %q", updatedSel.choice)
	}
	if cmd == nil {
		t.Error("expected quit command on enter")
	}
	if !updatedSel.done {
		t.Error("expected done=true after enter")
	}
}

func TestSelector_UpdateEnter_SecondTime_StillQuits_NoChange(t *testing.T) {
	sel, _ := newTestSelector("x", "y")
	sel.cursor = 1
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	afterFirst := m1.(Selector)
	m2, cmd2 := afterFirst.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	afterSecond := m2.(Selector)

	if afterSecond.choice != afterFirst.choice {
		t.Errorf("expected choice unchanged, got %q vs %q", afterSecond.choice, afterFirst.choice)
	}
	if cmd2 == nil {
		t.Error("expected quit command on second enter too")
	}
}

func TestSelector_UpdateCtrlC_Quits(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, cmd := sel.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector")
	}
	if !updatedSel.QuitRequested() {
		t.Error("expected quitRequested=true on ctrl+c")
	}
	if cmd == nil {
		t.Error("expected quit command on ctrl+c")
	}
}

func TestSelector_EnterWithEmptyOptions_NoPanic(t *testing.T) {
	sel, _ := newTestSelector()
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updatedSel := updatedModel.(Selector)
	if updatedSel.done {
		t.Error("expected done=false with empty options")
	}
	if updatedSel.choice != "" {
		t.Errorf("expected empty choice, got %q", updatedSel.choice)
	}
}

func TestSelector_UpdateEsc_Backs(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, cmd := sel.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updatedSel := updatedModel.(Selector)
	if !updatedSel.BackRequested() {
		t.Error("expected backRequested=true on esc")
	}
	if cmd == nil {
		t.Error("expected quit command on esc")
	}
}

func TestSelector_View_Normal_HighlightsCursor(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	sel.cursor = 0
	view := sel.View().Content

	if !strings.Contains(view, sel.placeholder) {
		t.Errorf("view should contain placeholder %q", sel.placeholder)
	}
	if !strings.Contains(view, "> client mode") {
		t.Errorf("expected highlighted row marker in view, got %q", view)
	}
}

func TestSelector_View_Done_IsEmpty(t *testing.T) {
	sel, _ := newTestSelector("a")
	sel.done = true
	if v := sel.View().Content; v != "" {
		t.Errorf("expected empty view when done, got %q", v)
	}
}

func TestSelector_Choice(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	if sel.Choice() != "" {
		t.Errorf("expected empty choice initially, got %q", sel.Choice())
	}
	sel.choice = "client"
	if sel.Choice() != "client" {
		t.Errorf("expected 'client', got %q", sel.Choice())
	}
}

func TestSplitPlaceholder_Multiline(t *testing.T) {
	title, details := splitPlaceholder("Configuration error\nReason: invalid port\nChoose another one")
	if title != "Configuration error" {
		t.Fatalf("unexpected title: %q", title)
	}
	if len(details) != 2 {
		t.Fatalf("unexpected details len: %d", len(details))
	}
}

func TestSelector_TabSwitchesToSettings(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	updatedSel := updatedModel.(Selector)

	view := updatedSel.View().Content
	if !strings.Contains(view, "Settings") {
		t.Fatalf("expected settings screen, got view: %q", view)
	}
}

func TestSelector_TabSwitchesToLogs(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := m2.(Selector).View().Content

	if !strings.Contains(view, "Logs") {
		t.Fatalf("expected logs screen, got view: %q", view)
	}
}

func TestSelector_TabSwitchesBackToMain(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m3, _ := m2.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := m3.(Selector).View().Content
	if !strings.Contains(view, "Main") {
		t.Fatalf("expected main screen after third tab, got %q", view)
	}
}

func TestSelector_TabSwitch_DoesNotRequestClearScreenCmd(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	updatedModel, cmd := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	if cmd != nil {
		t.Fatal("expected no command on tab switch to settings")
	}
	updated := updatedModel.(Selector)

	_, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	if cmd == nil {
		t.Fatal("expected logs update command on tab switch to logs")
	}
}

func TestSelector_LogsView_EmptyFeedShowsNoLogsYet(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := m2.(Selector).View().Content

	if !strings.Contains(view, "No logs yet") {
		t.Fatalf("expected empty logs hint, got view: %q", view)
	}
}

func TestSelector_SettingsToggleFooter(t *testing.T) {
	s := testSettings()
	p := s.Preferences()
	p.Theme = ThemeDark
	p.Language = "en"
	p.StatsUnits = StatsUnitsBiBytes
	p.ShowDataplaneStats = true
	p.ShowDataplaneGraph = true
	p.ShowFooter = true
	s.update(p)

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.settings = s
	sel.preferences = s.Preferences()
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})             // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyDown})  // stats units row
	m3, _ := m2.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyDown})  // dataplane stats row
	m4, _ := m3.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyDown})  // dataplane graph row
	m5, _ := m4.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyDown})  // footer row
	m6, _ := m5.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyRight}) // toggle
	toggled := m6.(Selector)

	if s.Preferences().ShowFooter {
		t.Fatalf("expected ShowFooter to be toggled off")
	}
	if toggled.preferences.ShowFooter {
		t.Fatalf("expected model ShowFooter to be toggled off")
	}
}

func TestSelector_SettingsToggleStatsUnits(t *testing.T) {
	s := testSettings()
	p := s.Preferences()
	p.Theme = ThemeDark
	p.Language = "en"
	p.StatsUnits = StatsUnitsBiBytes
	p.ShowDataplaneStats = true
	p.ShowDataplaneGraph = true
	p.ShowFooter = true
	s.update(p)

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.settings = s
	sel.preferences = s.Preferences()
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})             // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyDown})  // stats units row
	m3, _ := m2.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyRight}) // toggle
	toggled := m3.(Selector)

	if s.Preferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected StatsUnits to be toggled to bytes")
	}
	if toggled.preferences.StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected model StatsUnits to be toggled to bytes")
	}
}

func TestSelector_SettingsNavigationBoundsAndMutations(t *testing.T) {
	s := testSettings()
	p := s.Preferences()
	p.Theme = ThemeLight
	p.Language = "en"
	p.StatsUnits = StatsUnitsBytes
	p.ShowDataplaneStats = true
	p.ShowDataplaneGraph = true
	p.ShowFooter = true
	s.update(p)

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.settings = s
	sel.preferences = s.Preferences()
	sel.screen = selectorScreenSettings

	// Up at top stays at top.
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	sel = m1.(Selector)
	if sel.settingsCursor != 0 {
		t.Fatalf("expected cursor at top, got %d", sel.settingsCursor)
	}
	sel.settingsCursor = 1
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	sel = m1.(Selector)
	if sel.settingsCursor != 0 {
		t.Fatalf("expected up from row 1 to row 0, got %d", sel.settingsCursor)
	}

	// Move to bottom and verify lower bound.
	wantBottom := settingsVisibleRowCount(sel.preferences, false) - 1
	for i := 0; i < settingsVisibleRowCount(sel.preferences, false)+1; i++ {
		m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		sel = m1.(Selector)
	}
	if sel.settingsCursor != wantBottom {
		t.Fatalf("expected cursor at bottom, got %d", sel.settingsCursor)
	}

	// Theme row left wraps to dark.
	sel.settingsCursor = settingsThemeRow
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	sel = m1.(Selector)
	wantTheme := orderedThemeOptions[len(orderedThemeOptions)-1]
	if s.Preferences().Theme != wantTheme {
		t.Fatalf("expected theme %q after left wrap, got %q", wantTheme, s.Preferences().Theme)
	}

	// Stats row left toggles to bibytes.
	sel.settingsCursor = settingsStatsUnitsRow
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	sel = m1.(Selector)
	if s.Preferences().StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected bibytes after left toggle, got %q", s.Preferences().StatsUnits)
	}
	if sel.preferences.StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected model bibytes after left toggle, got %q", sel.preferences.StatsUnits)
	}

	// Dataplane stats row select toggles.
	sel.settingsCursor = settingsDataplaneStatsRow
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sel = m1.(Selector)
	if s.Preferences().ShowDataplaneStats {
		t.Fatalf("expected dataplane stats OFF after toggle")
	}
	if sel.preferences.ShowDataplaneStats {
		t.Fatalf("expected model dataplane stats OFF after toggle")
	}

	// Dataplane graph row select toggles.
	sel.settingsCursor = settingsDataplaneGraphRow
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sel = m1.(Selector)
	if s.Preferences().ShowDataplaneGraph {
		t.Fatalf("expected dataplane graph OFF after toggle")
	}
	if sel.preferences.ShowDataplaneGraph {
		t.Fatalf("expected model dataplane graph OFF after toggle")
	}

	// Footer row select toggles.
	sel.settingsCursor = settingsFooterRow
	m1, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sel = m1.(Selector)
	if s.Preferences().ShowFooter {
		t.Fatalf("expected footer OFF after toggle")
	}
	if sel.preferences.ShowFooter {
		t.Fatalf("expected model footer OFF after toggle")
	}
}

func TestSelector_ViewIncludesSubtitleAndDetails(t *testing.T) {
	col := &mockColorizer{}
	sel := NewSelector(
		"Title\nSubtitle\nDetail line",
		[]string{"one"},
		col,
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	view := sel.View().Content
	if !strings.Contains(view, "Subtitle") || !strings.Contains(view, "Detail line") {
		t.Fatalf("expected subtitle/details in view, got %q", view)
	}
}

func TestNextStatsUnits_UnknownCurrentFallsBackToBytes(t *testing.T) {
	got := nextStatsUnits(StatsUnitsOption("unexpected"), 1)
	if got != StatsUnitsBiBytes {
		t.Fatalf("expected fallback step from bytes to bibytes, got %q", got)
	}
}

func TestSelector_SettingsAndLogsView_WithWidth(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(8)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	_, _ = feed.Write([]byte("line one\n"))

	sel, _ := newTestSelector("a")
	sel.width = 100
	sel.height = 30
	sel.screen = selectorScreenSettings
	settings := sel.settingsView([]string{"detail"})
	if !strings.Contains(settings, "detail") || !strings.Contains(settings, "Theme") {
		t.Fatalf("expected settings details and rows, got %q", settings)
	}

	sel.screen = selectorScreenLogs
	sel.refreshLogsViewport()
	logs := sel.logsView()
	if !strings.Contains(logs, "line one") {
		t.Fatalf("expected runtime line in logs view, got %q", logs)
	}
}

func TestSelector_SettingsThemeChange_RequestsClearScreen(t *testing.T) {
	s := testSettings()
	p := s.Preferences()
	p.Theme = ThemeLight
	p.StatsUnits = StatsUnitsBytes
	p.ShowDataplaneStats = true
	p.ShowDataplaneGraph = true
	p.ShowFooter = true
	s.update(p)

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.settings = s
	sel.preferences = s.Preferences()
	sel.screen = selectorScreenSettings
	sel.settingsCursor = settingsThemeRow

	updatedModel, cmd := sel.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := updatedModel.(Selector)
	if cmd == nil {
		t.Fatal("expected clear-screen command when theme changes")
	}
	if updated.preferences.Theme != ThemeDark {
		t.Fatalf("expected theme to change to dark, got %q", updated.preferences.Theme)
	}
}

func TestSelector_View_TruncatesLongOptionToContentWidth(t *testing.T) {
	longOption := strings.Repeat("x", 160)
	sel, _ := newTestSelector(longOption)
	updatedModel, _ := sel.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	updatedSel := updatedModel.(Selector)

	view := updatedSel.View().Content
	if strings.Contains(view, longOption) {
		t.Fatalf("expected long option to be truncated in view")
	}
	if !strings.Contains(view, "...") {
		t.Fatalf("expected truncated marker in view, got: %q", view)
	}
}

func TestSelectorKeyMap_HelpLayouts(t *testing.T) {
	keys := defaultSelectorKeyMap()
	if len(keys.ShortHelp()) == 0 {
		t.Fatal("expected short help bindings")
	}
	full := keys.FullHelp()
	if len(full) != 2 || len(full[0]) == 0 || len(full[1]) == 0 {
		t.Fatalf("unexpected full help layout: %v", full)
	}
}

func TestSelector_UpdateWindowSizeAndQuestionMarkNoOp(t *testing.T) {
	sel, _ := newTestSelector("a", "b")
	m1, _ := sel.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	updated := m1.(Selector)
	if updated.width != 80 || updated.height != 20 {
		t.Fatalf("expected window size to be stored, got width=%d height=%d", updated.width, updated.height)
	}

	m2, _ := updated.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	afterQuestion := m2.(Selector)
	if afterQuestion.done {
		t.Fatal("expected '?' key to have no side effects")
	}
	if afterQuestion.cursor != updated.cursor {
		t.Fatalf("expected cursor unchanged, got %d -> %d", updated.cursor, afterQuestion.cursor)
	}
}

func TestSelector_NextThemeAndOnOffBranches(t *testing.T) {
	if got := nextTheme(ThemeLight, 1); got != ThemeDark {
		t.Fatalf("expected light->dark, got %q", got)
	}
	if got := nextTheme(ThemeDark, -1); got != ThemeLight {
		t.Fatalf("expected dark->light, got %q", got)
	}
	if got := onOff(false); got != "OFF" {
		t.Fatalf("expected OFF, got %q", got)
	}
}

func TestSplitPlaceholder_EmptyAndSingle(t *testing.T) {
	title, details := splitPlaceholder(" \n\t ")
	if title != "Choose option" || details != nil {
		t.Fatalf("expected fallback title, got title=%q details=%v", title, details)
	}
	title, details = splitPlaceholder("Only title")
	if title != "Only title" || details != nil {
		t.Fatalf("expected single title only, got title=%q details=%v", title, details)
	}
}

func TestSelector_LogsTail_WithGlobalFeed(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)

	EnableGlobalRuntimeLogCapture(8)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	_, _ = feed.Write([]byte("line one\nline two\n"))

	sel, _ := newTestSelector("Main title", "a")
	sel.height = 24
	lines := sel.logsTail()
	if len(lines) == 0 {
		t.Fatal("expected non-empty logs tail from global feed")
	}
}

func TestSelector_LogsViewportScrollAndFollowToggle(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	for i := 0; i < 40; i++ {
		_, _ = feed.Write([]byte(fmt.Sprintf("line-%02d\n", i)))
	}

	sel, _ := newTestSelector("Main title", "a", "b")
	updatedModel, _ := sel.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	sel = updatedModel.(Selector)
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	sel = updatedModel.(Selector)
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	sel = updatedModel.(Selector)

	if !sel.logViewport.AtBottom() {
		t.Fatal("expected selector logs viewport to start at tail")
	}

	beforeOffset := sel.logViewport.YOffset()
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	sel = updatedModel.(Selector)
	if sel.logFollow {
		t.Fatal("expected follow disabled after manual scroll in logs tab")
	}
	if sel.logViewport.YOffset() >= beforeOffset {
		t.Fatalf("expected viewport offset to move up, before=%d after=%d", beforeOffset, sel.logViewport.YOffset())
	}

	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	sel = updatedModel.(Selector)
	if !sel.logFollow {
		t.Fatal("expected follow enabled after pressing space")
	}
	if !sel.logViewport.AtBottom() {
		t.Fatal("expected viewport to jump to tail after enabling follow")
	}
}

func TestSelectorLogTickCmd_EmitsMessage(t *testing.T) {
	if _, ok := selectorLogTickCmd(1)().(selectorLogTickMsg); !ok {
		t.Fatal("expected selectorLogTickMsg from selector log tick command")
	}
}

func TestSelector_UpdateLogs_AllNavigationKeys(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	for i := 0; i < 40; i++ {
		_, _ = feed.Write([]byte(fmt.Sprintf("line-%02d\n", i)))
	}

	sel, _ := newTestSelector("Main title", "a", "b")
	updatedModel, _ := sel.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	sel = updatedModel.(Selector)
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // settings
	sel = updatedModel.(Selector)
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	sel = updatedModel.(Selector)

	// PgUp
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	sel = updatedModel.(Selector)
	if sel.logFollow {
		t.Fatal("expected logFollow=false after PgUp")
	}

	// PgDown when at bottom
	sel.logViewport.GotoBottom()
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	sel = updatedModel.(Selector)
	if !sel.logFollow {
		t.Fatal("expected logFollow=true after PgDown at bottom")
	}

	// Home
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	sel = updatedModel.(Selector)
	if sel.logFollow {
		t.Fatal("expected logFollow=false after Home")
	}

	// End
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	sel = updatedModel.(Selector)
	if !sel.logFollow {
		t.Fatal("expected logFollow=true after End")
	}

	// Down when not at bottom
	sel.logViewport.GotoTop()
	sel.logFollow = false
	updatedModel, _ = sel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	sel = updatedModel.(Selector)
	if sel.logFollow {
		t.Fatal("expected logFollow=false after Down when not at bottom")
	}
}

func TestSelector_EnsureLogsViewport_WhenLogReadyFalse(t *testing.T) {
	sel, _ := newTestSelector("a", "b")
	sel.logReady = false
	sel.width = 100
	sel.height = 30

	sel.ensureLogsViewport()
	if !sel.logReady {
		t.Fatal("expected logReady=true after ensureLogsViewport")
	}
	if sel.logViewport.Width() <= 0 {
		t.Fatalf("expected viewport width > 0, got %d", sel.logViewport.Width())
	}
}

func TestSelectorLogUpdateCmd_PlainFeedFallsBackToTick(t *testing.T) {
	feed := testRuntimeLogFeed{lines: []string{"line"}}
	stop := make(chan struct{})
	cmd := selectorLogUpdateCmd(feed, stop, 1)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(selectorLogTickMsg); !ok {
		t.Fatalf("expected selectorLogTickMsg from plain feed fallback, got %T", msg)
	}
}

func TestSelectorLogUpdateCmd_ChangeFeedNilChanges(t *testing.T) {
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            nil,
	}
	stop := make(chan struct{})
	cmd := selectorLogUpdateCmd(feed, stop, 1)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(selectorLogTickMsg); !ok {
		t.Fatalf("expected selectorLogTickMsg from nil Changes fallback, got %T", msg)
	}
}

func TestSelector_TabCycleMainSettingsLogsMain_ReturnsLogCmd(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")

	// Tab: main -> settings (no cmd)
	m1, cmd1 := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s1 := m1.(Selector)
	if s1.screen != selectorScreenSettings {
		t.Fatalf("expected settings screen, got %v", s1.screen)
	}
	if cmd1 != nil {
		t.Fatal("expected no cmd when entering settings")
	}

	// Tab: settings -> logs (should return cmd)
	m2, cmd2 := s1.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s2 := m2.(Selector)
	if s2.screen != selectorScreenLogs {
		t.Fatalf("expected logs screen, got %v", s2.screen)
	}
	if cmd2 == nil {
		t.Fatal("expected log update cmd when entering logs screen")
	}

	// Tab: logs -> main (no cmd, but logWait should be stopped)
	m3, cmd3 := s2.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s3 := m3.(Selector)
	if s3.screen != selectorScreenMain {
		t.Fatalf("expected main screen, got %v", s3.screen)
	}
	if cmd3 != nil {
		t.Fatal("expected no cmd when returning to main")
	}
}

func TestSelector_WindowSizeMsgOnLogsScreen(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	for i := 0; i < 20; i++ {
		_, _ = feed.Write([]byte(fmt.Sprintf("line-%02d\n", i)))
	}

	sel, _ := newTestSelector("Main title", "a", "b")
	// Navigate to logs screen.
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})           // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	s := m2.(Selector)

	// Send WindowSizeMsg on logs screen.
	m3, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	s3 := m3.(Selector)
	if s3.width != 100 || s3.height != 30 {
		t.Fatalf("expected updated size, got %dx%d", s3.width, s3.height)
	}
	// The logs viewport should have been refreshed.
	if s3.logViewport.TotalLineCount() == 0 {
		t.Fatal("expected logs viewport content after WindowSizeMsg on logs screen")
	}
}

func TestSelector_LogTickMatchingSeqOnLogsScreen(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	_, _ = feed.Write([]byte("log line\n"))

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})           // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	s := m2.(Selector)

	// Send a matching selectorLogTickMsg.
	m3, cmd := s.Update(selectorLogTickMsg{seq: s.logTickSeq})
	if cmd == nil {
		t.Fatal("expected follow-up log cmd on matching log tick")
	}
	_ = m3.(Selector)
}

func TestSelector_LogTickStaleSeqIgnored(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyTab})           // settings
	m2, _ := m1.(Selector).Update(tea.KeyPressMsg{Code: tea.KeyTab}) // logs
	s := m2.(Selector)

	// Send a stale selectorLogTickMsg (seq doesn't match).
	_, cmd := s.Update(selectorLogTickMsg{seq: s.logTickSeq + 99})
	if cmd != nil {
		t.Fatal("expected nil cmd for stale log tick seq")
	}
}

func TestSelector_LogTickOnNonLogsScreenIgnored(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	// Stay on main screen (not logs).
	_, cmd := sel.Update(selectorLogTickMsg{seq: sel.logTickSeq})
	if cmd != nil {
		t.Fatal("expected nil cmd for log tick on non-logs screen")
	}
}

func TestNextTheme_UnknownTheme_DefaultsToIdx0(t *testing.T) {
	unknown := ThemeOption("totally_unknown")
	got := nextTheme(unknown, 1)
	// Unknown should default to idx=0, then step forward to idx=1.
	if got != orderedThemeOptions[1] {
		t.Fatalf("expected %q for unknown+step(1), got %q", orderedThemeOptions[1], got)
	}
	got = nextTheme(unknown, -1)
	// Unknown defaults to idx=0, step back wraps to last.
	if got != orderedThemeOptions[len(orderedThemeOptions)-1] {
		t.Fatalf("expected %q for unknown+step(-1), got %q", orderedThemeOptions[len(orderedThemeOptions)-1], got)
	}
}

func TestSelector_RefreshLogsViewport_SetYOffsetFallback(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	for i := 0; i < 40; i++ {
		_, _ = feed.Write([]byte(fmt.Sprintf("line-%02d\n", i)))
	}

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.width = 120
	sel.height = 24

	// Populate viewport initially.
	sel.refreshLogsViewport()
	// Scroll up so we are not at bottom, and disable follow.
	sel.logViewport.GotoTop()
	sel.logFollow = false

	// Set a known offset and refresh again.
	sel.logViewport.SetYOffset(2)
	savedOffset := sel.logViewport.YOffset()

	sel.refreshLogsViewport()
	// logFollow is false and was not at bottom, so it should use SetYOffset fallback.
	if sel.logViewport.YOffset() != savedOffset {
		t.Fatalf("expected viewport offset to be restored to %d, got %d", savedOffset, sel.logViewport.YOffset())
	}
	if sel.logFollow {
		t.Fatal("expected logFollow to remain false when not at bottom")
	}
}

func TestSelectorLogUpdateCmd_StopClosedReturnsTick(t *testing.T) {
	changes := make(chan struct{}, 1)
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            changes,
	}
	stop := make(chan struct{})
	close(stop) // close immediately

	cmd := selectorLogUpdateCmd(feed, stop, 42)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	tick, ok := msg.(selectorLogTickMsg)
	if !ok {
		t.Fatalf("expected selectorLogTickMsg when stop is closed, got %T", msg)
	}
	// When stop fires, seq should be zero (not the passed-in seq).
	if tick.seq != 0 {
		t.Fatalf("expected seq=0 from stop branch, got %d", tick.seq)
	}
}

func TestSelectorLogUpdateCmd_ChangeFeedSignalReturnsMatchingSeq(t *testing.T) {
	changes := make(chan struct{}, 1)
	changes <- struct{}{} // signal immediately
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            changes,
	}
	stop := make(chan struct{})

	cmd := selectorLogUpdateCmd(feed, stop, 42)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	tick, ok := msg.(selectorLogTickMsg)
	if !ok {
		t.Fatalf("expected selectorLogTickMsg from changes signal, got %T", msg)
	}
	if tick.seq != 42 {
		t.Fatalf("expected seq=42 from changes signal, got %d", tick.seq)
	}
}

func TestSelector_DownKeyAtBottom_SetsFollowTrue(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)
	EnableGlobalRuntimeLogCapture(64)
	feed := GlobalRuntimeLogFeed().(*RuntimeLogBuffer)
	// Write a single line so viewport is at bottom.
	_, _ = feed.Write([]byte("single line\n"))

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.width = 120
	sel.height = 24
	sel.screen = selectorScreenLogs
	sel.refreshLogsViewport()
	sel.logViewport.GotoBottom()
	sel.logFollow = false

	updatedModel, _ := sel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated := updatedModel.(Selector)
	if !updated.logFollow {
		t.Fatal("expected logFollow=true when Down key pressed and viewport is already at bottom")
	}
}
