package bubble_tea

import (
	"fmt"
	"strings"
	"testing"

	"tungo/presentation/ui/tui/internal/ui/value_objects"

	tea "github.com/charmbracelet/bubbletea"
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
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyUp})
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
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyUp})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateDown(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyDown})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateDown_AtBottom_NoChange(t *testing.T) {
	sel, _ := newTestSelector("a", "b")
	sel.cursor = 1
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyDown})
	updatedSel := updatedModel.(Selector)
	if updatedSel.cursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", updatedSel.cursor)
	}
}

func TestSelector_UpdateEnter_FirstTime_SetsChoice_Quits(t *testing.T) {
	sel, _ := newTestSelector("client", "server")
	sel.cursor = 0
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	afterFirst := m1.(Selector)
	m2, cmd2 := afterFirst.Update(tea.KeyMsg{Type: tea.KeyEnter})
	afterSecond := m2.(Selector)

	if afterSecond.choice != afterFirst.choice {
		t.Errorf("expected choice unchanged, got %q vs %q", afterSecond.choice, afterFirst.choice)
	}
	if cmd2 == nil {
		t.Error("expected quit command on second enter too")
	}
}

func TestSelector_UpdateQ_Quits(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	updatedSel, ok := updatedModel.(Selector)
	if !ok {
		t.Fatal("Update did not return Selector")
	}
	if !updatedSel.QuitRequested() {
		t.Error("expected quitRequested=true on 'q'")
	}
	if cmd == nil {
		t.Error("expected quit command on 'q'")
	}
}

func TestSelector_UpdateEsc_Backs(t *testing.T) {
	sel, _ := newTestSelector("client mode", "server mode")
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
	view := sel.View()

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
	if v := sel.View(); v != "" {
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
	updatedModel, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab})
	updatedSel := updatedModel.(Selector)

	view := updatedSel.View()
	if !strings.Contains(view, "Settings") {
		t.Fatalf("expected settings screen, got view: %q", view)
	}
}

func TestSelector_TabSwitchesToLogs(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m2.(Selector).View()

	if !strings.Contains(view, "Logs") {
		t.Fatalf("expected logs screen, got view: %q", view)
	}
}

func TestSelector_TabSwitchesBackToMain(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyTab})
	m3, _ := m2.(Selector).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m3.(Selector).View()
	if !strings.Contains(view, "Main") {
		t.Fatalf("expected main screen after third tab, got %q", view)
	}
}

func TestSelector_TabSwitch_DoesNotRequestClearScreenCmd(t *testing.T) {
	sel, _ := newTestSelector("Main title", "a", "b")
	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	if cmd != nil {
		t.Fatal("expected no command on tab switch to settings")
	}
	updated := updatedModel.(Selector)

	_, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	if cmd == nil {
		t.Fatal("expected logs update command on tab switch to logs")
	}
}

func TestSelector_LogsView_EmptyFeedShowsNoLogsYet(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m2.(Selector).View()

	if !strings.Contains(view, "No logs yet") {
		t.Fatalf("expected empty logs hint, got view: %q", view)
	}
}

func TestSelector_SettingsToggleFooter(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeDark
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab})            // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m3, _ := m2.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // footer row
	m4 := m3
	_, _ = m4.(Selector).Update(tea.KeyMsg{Type: tea.KeyRight}) // toggle

	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected ShowFooter to be toggled off")
	}
}

func TestSelector_SettingsToggleStatsUnits(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeDark
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab})            // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m3 := m2
	_, _ = m3.(Selector).Update(tea.KeyMsg{Type: tea.KeyRight}) // toggle

	if CurrentUIPreferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected StatsUnits to be toggled to bytes")
	}
}

func TestSelector_SettingsNavigationBoundsAndMutations(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.Language = "en"
		p.StatsUnits = StatsUnitsBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeLight
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.screen = selectorScreenSettings

	// Up at top stays at top.
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyUp})
	sel = m1.(Selector)
	if sel.settingsCursor != 0 {
		t.Fatalf("expected cursor at top, got %d", sel.settingsCursor)
	}
	sel.settingsCursor = 1
	m1, _ = sel.Update(tea.KeyMsg{Type: tea.KeyUp})
	sel = m1.(Selector)
	if sel.settingsCursor != 0 {
		t.Fatalf("expected up from row 1 to row 0, got %d", sel.settingsCursor)
	}

	// Move to bottom and verify lower bound.
	for i := 0; i < settingsRowsCount+1; i++ {
		m1, _ = sel.Update(tea.KeyMsg{Type: tea.KeyDown})
		sel = m1.(Selector)
	}
	if sel.settingsCursor != settingsRowsCount-1 {
		t.Fatalf("expected cursor at bottom, got %d", sel.settingsCursor)
	}

	// Theme row left wraps to dark.
	sel.settingsCursor = settingsThemeRow
	m1, _ = sel.Update(tea.KeyMsg{Type: tea.KeyLeft})
	sel = m1.(Selector)
	wantTheme := orderedThemeOptions[len(orderedThemeOptions)-1]
	if CurrentUIPreferences().Theme != wantTheme {
		t.Fatalf("expected theme %q after left wrap, got %q", wantTheme, CurrentUIPreferences().Theme)
	}

	// Stats row left toggles to bibytes.
	sel.settingsCursor = settingsStatsUnitsRow
	_, _ = sel.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if CurrentUIPreferences().StatsUnits != StatsUnitsBiBytes {
		t.Fatalf("expected bibytes after left toggle, got %q", CurrentUIPreferences().StatsUnits)
	}

	// Footer row select toggles.
	sel.settingsCursor = settingsFooterRow
	_, _ = sel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected footer OFF after toggle")
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
	view := sel.View()
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
	settings := sel.settingsView("ignored", "ignored", []string{"detail"})
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
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.StatsUnits = StatsUnitsBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeLight
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	sel.screen = selectorScreenSettings
	sel.settingsCursor = settingsThemeRow

	updatedModel, cmd := sel.Update(tea.KeyMsg{Type: tea.KeyRight})
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

	view := updatedSel.View()
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

	m2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
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
	updatedModel, _ = sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	sel = updatedModel.(Selector)
	updatedModel, _ = sel.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	sel = updatedModel.(Selector)

	if !sel.logViewport.AtBottom() {
		t.Fatal("expected selector logs viewport to start at tail")
	}

	beforeOffset := sel.logViewport.YOffset
	updatedModel, _ = sel.Update(tea.KeyMsg{Type: tea.KeyUp})
	sel = updatedModel.(Selector)
	if sel.logFollow {
		t.Fatal("expected follow disabled after manual scroll in logs tab")
	}
	if sel.logViewport.YOffset >= beforeOffset {
		t.Fatalf("expected viewport offset to move up, before=%d after=%d", beforeOffset, sel.logViewport.YOffset)
	}

	updatedModel, _ = sel.Update(tea.KeyMsg{Type: tea.KeySpace})
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
