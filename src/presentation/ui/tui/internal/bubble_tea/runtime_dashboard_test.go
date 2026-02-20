package bubble_tea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"tungo/infrastructure/telemetry/trafficstats"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRuntimeDashboard_TabSwitchesToSettings(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := updated.(RuntimeDashboard).View()

	if !strings.Contains(view, "Settings") {
		t.Fatalf("expected settings screen after Tab, got view: %q", view)
	}
}

func TestNewRuntimeDashboard_DefaultsNilContextAndMode(t *testing.T) {
	m := NewRuntimeDashboard(nil, RuntimeDashboardOptions{})
	if m.ctx == nil {
		t.Fatal("expected fallback context when nil is passed")
	}
	if m.mode != RuntimeDashboardClient {
		t.Fatalf("expected default client mode, got %q", m.mode)
	}
}

func TestRuntimeDashboard_TabSwitchesToLogs(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m2.(RuntimeDashboard).View()

	if !strings.Contains(view, "Logs") {
		t.Fatalf("expected logs screen after second Tab, got view: %q", view)
	}
}

func TestRuntimeDashboard_TabSwitchesBackToDataplane(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyTab})
	m3, _ := m2.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m3.(RuntimeDashboard).View()

	if !strings.Contains(view, "Status: Connected") {
		t.Fatalf("expected dataplane screen after third Tab, got view: %q", view)
	}
}

func TestRuntimeDashboard_TabSwitch_DoesNotRequestClearScreenCmd(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	if cmd != nil {
		t.Fatal("expected no command on tab switch to settings")
	}
	updated := updatedModel.(RuntimeDashboard)

	_, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	if cmd == nil {
		t.Fatal("expected logs update command on tab switch to logs")
	}
}

func TestRuntimeDashboard_TogglesFooterInSettings(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowDataplaneStats = true
		p.ShowDataplaneGraph = true
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeDark
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})                      // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m3, _ := m2.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // dataplane stats row
	m4, _ := m3.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // dataplane graph row
	m5, _ := m4.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // footer row
	m6 := m5
	_, _ = m6.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyRight}) // toggle

	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected ShowFooter to be toggled off")
	}
}

func TestRuntimeDashboard_TogglesStatsUnitsInSettings(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeDark
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowDataplaneStats = true
		p.ShowDataplaneGraph = true
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeDark
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})                      // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m3 := m2
	_, _ = m3.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyRight}) // toggle

	if CurrentUIPreferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected StatsUnits to be toggled to bytes")
	}
}

type testRuntimeProgram struct {
	run func() (tea.Model, error)
}

func (p testRuntimeProgram) Run() (tea.Model, error) {
	return p.run()
}

type testRuntimeLogFeed struct {
	lines []string
}

func (f testRuntimeLogFeed) Tail(limit int) []string {
	if limit <= 0 || len(f.lines) == 0 {
		return nil
	}
	if len(f.lines) <= limit {
		return append([]string(nil), f.lines...)
	}
	return append([]string(nil), f.lines[len(f.lines)-limit:]...)
}

func (f testRuntimeLogFeed) TailInto(dst []string, limit int) int {
	if limit <= 0 || len(dst) == 0 || len(f.lines) == 0 {
		return 0
	}
	if limit > len(dst) {
		limit = len(dst)
	}
	start := 0
	if len(f.lines) > limit {
		start = len(f.lines) - limit
	}
	return copy(dst, f.lines[start:])
}

type nonDashboardModel struct{}

func (nonDashboardModel) Init() tea.Cmd                           { return nil }
func (nonDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return nonDashboardModel{}, nil }
func (nonDashboardModel) View() string                            { return "x" }

func TestRunRuntimeDashboard_RunErrorWhenContextCanceled_IsIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	oldFactory := newRuntimeDashboardProgram
	t.Cleanup(func() { newRuntimeDashboardProgram = oldFactory })
	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				return model, errors.New("boom")
			},
		}
	}

	quit, err := RunRuntimeDashboard(ctx, RuntimeDashboardOptions{})
	if err != nil {
		t.Fatalf("expected nil error when context already canceled, got %v", err)
	}
	if quit {
		t.Fatal("expected quit=false")
	}
}

func TestRunRuntimeDashboard_RunErrorReturnedWhenContextActive(t *testing.T) {
	oldFactory := newRuntimeDashboardProgram
	t.Cleanup(func() { newRuntimeDashboardProgram = oldFactory })
	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				return model, errors.New("boom")
			},
		}
	}

	_, err := RunRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	if err == nil {
		t.Fatal("expected run error to be returned")
	}
}

func TestRunRuntimeDashboard_FinalModelTypeAndFlags(t *testing.T) {
	oldFactory := newRuntimeDashboardProgram
	t.Cleanup(func() { newRuntimeDashboardProgram = oldFactory })
	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				return nonDashboardModel{}, nil
			},
		}
	}
	quit, err := RunRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quit {
		t.Fatal("expected quit=false for non-dashboard final model")
	}

	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				m := model.(RuntimeDashboard)
				m.reconfigureRequested = true
				return m, nil
			},
		}
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !quit {
		t.Fatal("expected quit=true when final model requested quit")
	}

	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				m := model.(RuntimeDashboard)
				m.exitRequested = true
				return m, nil
			},
		}
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	if err == nil || !errors.Is(err, ErrRuntimeDashboardExitRequested) {
		t.Fatalf("expected ErrRuntimeDashboardExitRequested, got %v", err)
	}
	if quit {
		t.Fatal("expected quit=false on explicit exit request")
	}
}

func TestRunRuntimeDashboard_NilContext(t *testing.T) {
	oldFactory := newRuntimeDashboardProgram
	t.Cleanup(func() { newRuntimeDashboardProgram = oldFactory })
	newRuntimeDashboardProgram = func(model tea.Model) runtimeDashboardProgram {
		return testRuntimeProgram{
			run: func() (tea.Model, error) {
				return model, nil
			},
		}
	}

	quit, err := RunRuntimeDashboard(nil, RuntimeDashboardOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quit {
		t.Fatal("expected quit=false")
	}
}

func TestNewRuntimeDashboardProgram_DefaultFactory(t *testing.T) {
	program := newRuntimeDashboardProgram(NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{}))
	if program == nil {
		t.Fatal("expected non-nil runtime dashboard program")
	}
}

func TestRuntimeDashboard_InitAndTickCommands(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	if cmd := m.Init(); cmd == nil {
		t.Fatal("expected init batch command")
	}
	if cmd := runtimeTickCmd(1); cmd == nil {
		t.Fatal("expected runtimeTickCmd command")
	}
	if cmd := runtimeLogTickCmd(1); cmd == nil {
		t.Fatal("expected runtimeLogTickCmd command")
	}
}

func TestRuntimeDashboard_Update_WindowAndContextDoneAndQuit(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: []string{"one", "two"}},
	})
	updatedModel, cmd := m.Update(runtimeTickMsg{seq: m.tickSeq})
	if cmd == nil {
		t.Fatal("expected follow-up tick cmd on runtimeTickMsg")
	}
	updated := updatedModel.(RuntimeDashboard)

	updatedModel, cmd = updated.Update(runtimeLogTickMsg{seq: updated.logTickSeq})
	if cmd != nil {
		t.Fatal("expected no log tick cmd when logs screen is inactive")
	}
	updated = updatedModel.(RuntimeDashboard)
	if updated.logViewport.TotalLineCount() != 0 {
		t.Fatal("expected logs not refreshed while logs screen is inactive")
	}

	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	updated = updatedModel.(RuntimeDashboard)
	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	updated = updatedModel.(RuntimeDashboard)
	updatedModel, cmd = updated.Update(runtimeLogTickMsg{seq: updated.logTickSeq})
	if cmd == nil {
		t.Fatal("expected follow-up log tick cmd on runtimeLogTickMsg in logs screen")
	}
	updated = updatedModel.(RuntimeDashboard)

	updatedModel, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated = updatedModel.(RuntimeDashboard)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("unexpected size: %dx%d", updated.width, updated.height)
	}
	if updated.logViewport.TotalLineCount() == 0 {
		t.Fatal("expected logs refreshed on window size message")
	}

	updatedModel, cmd = updated.Update(runtimeContextDoneMsg{})
	if cmd == nil {
		t.Fatal("expected quit cmd on runtimeContextDoneMsg")
	}
	updated = updatedModel.(RuntimeDashboard)

	updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd on ctrl+c")
	}
	if !updatedModel.(RuntimeDashboard).exitRequested {
		t.Fatal("expected exitRequested flag on ctrl+c")
	}
}

func TestRuntimeDashboard_Update_IgnoresNonSettingsNavigationKeys(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	updated := updatedModel.(RuntimeDashboard)
	if updated.screen != runtimeScreenDataplane {
		t.Fatalf("expected dataplane screen to remain, got %v", updated.screen)
	}
	updatedModel, _ = updated.Update(struct{}{})
	if _, ok := updatedModel.(RuntimeDashboard); !ok {
		t.Fatalf("expected runtime dashboard model on unknown msg, got %T", updatedModel)
	}
}

func TestRuntimeDashboard_EscOnDataplane_OpensConfirm_StayCancels(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.confirmOpen {
		t.Fatal("expected confirm to open on esc in dataplane")
	}
	view := updated.View()
	if !strings.Contains(view, "Stop tunnel and reconfigure?") {
		t.Fatalf("expected confirm prompt in view, got %q", view)
	}

	updatedModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = updatedModel.(RuntimeDashboard)
	if cmd != nil {
		t.Fatal("expected no quit command when selecting Stay")
	}
	if updated.confirmOpen {
		t.Fatal("expected confirm to close after selecting Stay")
	}
	if updated.exitRequested || updated.reconfigureRequested {
		t.Fatal("did not expect exit or reconfigure flags on Stay")
	}
}

func TestRuntimeDashboard_EscOnDataplane_ConfirmReconfigureQuits(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(RuntimeDashboard)
	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 1 {
		t.Fatalf("expected confirm cursor on reconfigure option, got %d", updated.confirmCursor)
	}

	updatedModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = updatedModel.(RuntimeDashboard)
	if cmd == nil {
		t.Fatal("expected quit command when confirming reconfigure")
	}
	if !updated.reconfigureRequested {
		t.Fatal("expected reconfigureRequested=true when confirming reconfigure")
	}
	if updated.exitRequested {
		t.Fatal("did not expect exitRequested=true when confirming reconfigure")
	}
}

func TestRuntimeDashboard_EscOnSettingsAndLogs_NavigatesBack(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	updated := updatedModel.(RuntimeDashboard)
	if updated.screen != runtimeScreenSettings {
		t.Fatalf("expected settings screen, got %v", updated.screen)
	}
	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated = updatedModel.(RuntimeDashboard)
	if updated.screen != runtimeScreenDataplane {
		t.Fatalf("expected esc to navigate back to dataplane, got %v", updated.screen)
	}

	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	updated = updatedModel.(RuntimeDashboard)
	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	updated = updatedModel.(RuntimeDashboard)
	if updated.screen != runtimeScreenLogs {
		t.Fatalf("expected logs screen, got %v", updated.screen)
	}
	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated = updatedModel.(RuntimeDashboard)
	if updated.screen != runtimeScreenSettings {
		t.Fatalf("expected esc to navigate back to settings, got %v", updated.screen)
	}
}

func TestRuntimeDashboard_SettingsNavigationAndMutation(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowDataplaneStats = true
		p.ShowDataplaneGraph = true
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeLight
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.screen = runtimeScreenSettings

	// Up at top should stay at top.
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedModel.(RuntimeDashboard)
	if m.settingsCursor != 0 {
		t.Fatalf("expected cursor at top, got %d", m.settingsCursor)
	}
	m.settingsCursor = 1
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedModel.(RuntimeDashboard)
	if m.settingsCursor != 0 {
		t.Fatalf("expected up from row 1 to row 0, got %d", m.settingsCursor)
	}

	// Move to bottom, Down at bottom should stay there.
	for i := 0; i < settingsRowsCount+1; i++ {
		updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updatedModel.(RuntimeDashboard)
	}
	if m.settingsCursor != settingsRowsCount-1 {
		t.Fatalf("expected cursor at bottom, got %d", m.settingsCursor)
	}
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedModel.(RuntimeDashboard)
	if m.settingsCursor != settingsRowsCount-1 {
		t.Fatalf("expected cursor to stay at bottom, got %d", m.settingsCursor)
	}

	// Theme row: Left/Right.
	m.settingsCursor = settingsThemeRow
	m.preferences = CurrentUIPreferences()
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updatedModel.(RuntimeDashboard)
	if CurrentUIPreferences().Theme != ThemeDark {
		t.Fatalf("expected theme dark after right, got %q", CurrentUIPreferences().Theme)
	}
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updatedModel.(RuntimeDashboard)
	if CurrentUIPreferences().Theme != ThemeLight {
		t.Fatalf("expected theme light after left, got %q", CurrentUIPreferences().Theme)
	}

	// Stats units row: Enter toggles.
	m.settingsCursor = settingsStatsUnitsRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected stats units bytes, got %q", CurrentUIPreferences().StatsUnits)
	}

	// Dataplane stats row: Enter toggles.
	m.settingsCursor = settingsDataplaneStatsRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().ShowDataplaneStats {
		t.Fatalf("expected dataplane stats off after toggle")
	}

	// Dataplane graph row: Enter toggles.
	m.settingsCursor = settingsDataplaneGraphRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().ShowDataplaneGraph {
		t.Fatalf("expected dataplane graph off after toggle")
	}

	// Footer row: Enter toggles.
	m.settingsCursor = settingsFooterRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected footer off after toggle")
	}

	// Unmatched key leaves settings unchanged.
	prevCursor := m.settingsCursor
	updatedModel, _ = m.updateSettings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updatedModel.(RuntimeDashboard)
	if m.settingsCursor != prevCursor {
		t.Fatalf("expected cursor unchanged on unmatched key, got %d", m.settingsCursor)
	}
}

func TestRuntimeDashboard_MainView_ServerAndFooterOff(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.ShowDataplaneStats = true
		p.ShowDataplaneGraph = true
		p.ShowFooter = false
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		Mode: RuntimeDashboardServer,
	})
	m.width = 120
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Mode: Server") {
		t.Fatalf("expected server mode label, got %q", view)
	}
	if !strings.Contains(view, "Status: Running") {
		t.Fatalf("expected running status in main view, got %q", view)
	}
	if !strings.Contains(view, "Total RX") {
		t.Fatalf("expected traffic totals in dataplane view, got %q", view)
	}
	if !strings.Contains(view, "RX trend:") || !strings.Contains(view, "TX trend:") {
		t.Fatalf("expected sparkline trend lines in dataplane view, got %q", view)
	}
}

func TestRuntimeDashboard_MainView_CanHideStatsAndGraph(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.ShowDataplaneStats = false
		p.ShowDataplaneGraph = false
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.width = 120
	m.height = 30
	view := m.View()
	if strings.Contains(view, "Total RX") || strings.Contains(view, "RX trend:") || strings.Contains(view, "TX trend:") {
		t.Fatalf("expected stats and trend lines hidden, got %q", view)
	}
	if !strings.Contains(view, "Dataplane metrics are hidden in Settings.") {
		t.Fatalf("expected hidden-metrics hint, got %q", view)
	}
}

func TestRuntimeDashboard_RefreshLogsNilFeed(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.logViewport.SetContent("stale")
	m.refreshLogs()
	if !strings.Contains(m.logViewport.View(), "No logs yet") {
		t.Fatalf("expected no logs placeholder when feed is absent, got %q", m.logViewport.View())
	}
}

func TestWaitForRuntimeContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := waitForRuntimeContextDone(ctx)
	done := make(chan tea.Msg, 1)
	go func() {
		done <- cmd()
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case msg := <-done:
		if _, ok := msg.(runtimeContextDoneMsg); !ok {
			t.Fatalf("expected runtimeContextDoneMsg, got %T", msg)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("waitForRuntimeContextDone did not unblock after cancel")
	}
}

func TestRuntimeTickCommands_EmitMessages(t *testing.T) {
	if _, ok := runtimeLogTickCmd(1)().(runtimeLogTickMsg); !ok {
		t.Fatal("expected runtimeLogTickMsg")
	}
	if _, ok := runtimeTickCmd(1)().(runtimeTickMsg); !ok {
		t.Fatal("expected runtimeTickMsg")
	}
}

func TestRuntimeDashboard_SettingsAndLogsView_WithWidth(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: []string{"runtime log line"}},
	})
	m.width = 100
	m.height = 30
	m.screen = runtimeScreenSettings
	settingsView := m.View()
	if !strings.Contains(settingsView, "Theme") {
		t.Fatalf("expected settings rows in settings view, got %q", settingsView)
	}

	m.screen = runtimeScreenLogs
	m.refreshLogs()
	logsView := m.View()
	if !strings.Contains(logsView, "runtime log line") {
		t.Fatalf("expected log line in logs view, got %q", logsView)
	}
}

func TestRuntimeDashboard_SettingsThemeChange_RequestsClearScreen(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.StatsUnits = StatsUnitsBytes
		p.ShowDataplaneStats = true
		p.ShowDataplaneGraph = true
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeLight
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowDataplaneStats = true
			p.ShowDataplaneGraph = true
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.screen = runtimeScreenSettings
	m.settingsCursor = settingsThemeRow

	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := updatedModel.(RuntimeDashboard)
	if cmd == nil {
		t.Fatal("expected clear-screen command when runtime theme changes")
	}
	if updated.preferences.Theme != ThemeDark {
		t.Fatalf("expected theme to change to dark, got %q", updated.preferences.Theme)
	}
}

func TestRuntimeDashboard_LogsViewportScrollAndFollowToggle(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updatedModel.(RuntimeDashboard)

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m = updatedModel.(RuntimeDashboard)
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	m = updatedModel.(RuntimeDashboard)
	if !m.logViewport.AtBottom() {
		t.Fatal("expected logs viewport to follow tail by default")
	}

	beforeOffset := m.logViewport.YOffset
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedModel.(RuntimeDashboard)
	if m.logFollow {
		t.Fatal("expected follow mode disabled after manual up scroll")
	}
	if m.logViewport.YOffset >= beforeOffset {
		t.Fatalf("expected viewport offset to move up, before=%d after=%d", beforeOffset, m.logViewport.YOffset)
	}

	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updatedModel.(RuntimeDashboard)
	if !m.logFollow {
		t.Fatal("expected follow mode enabled after space toggle")
	}
	if !m.logViewport.AtBottom() {
		t.Fatal("expected viewport to jump to tail when follow mode is enabled")
	}
}

func TestRuntimeDashboard_RecordTrafficSample_RingWraps(t *testing.T) {
	var m RuntimeDashboard
	for i := 0; i < runtimeSparklinePoints+3; i++ {
		m.recordTrafficSample(trafficstats.Snapshot{
			RXRate: uint64(i + 1),
			TXRate: uint64(100 + i + 1),
		})
	}

	if m.sampleCount != runtimeSparklinePoints {
		t.Fatalf("expected full sample count %d, got %d", runtimeSparklinePoints, m.sampleCount)
	}
	if m.sampleCursor != 3 {
		t.Fatalf("expected sample cursor wrap to 3, got %d", m.sampleCursor)
	}
	if got := ringSampleAt(m.rxSamples, m.sampleCount, m.sampleCursor, 0); got != 4 {
		t.Fatalf("expected oldest sample to be 4 after wrap, got %d", got)
	}
	if got := ringSampleAt(m.rxSamples, m.sampleCount, m.sampleCursor, runtimeSparklinePoints-1); got != runtimeSparklinePoints+3 {
		t.Fatalf("expected newest sample to be %d, got %d", runtimeSparklinePoints+3, got)
	}
}

func TestRenderRateBrailleRing_EmptyAndZeroSeries(t *testing.T) {
	empty := renderRateBrailleRing([runtimeSparklinePoints]uint64{}, 0, 0, 12)
	if empty != "no-data" {
		t.Fatalf("expected no-data marker, got %q", empty)
	}

	var zero [runtimeSparklinePoints]uint64
	out := renderRateBrailleRing(zero, 8, 8, 8)
	if utf8.RuneCountInString(out) != 8 {
		t.Fatalf("expected braille width 8, got %d (%q)", utf8.RuneCountInString(out), out)
	}
	if out != strings.Repeat("⣀", 8) {
		t.Fatalf("expected flat zero braille trend, got %q", out)
	}
}

func TestRenderRateBrailleRing_NonFlatUsesMultipleGlyphs(t *testing.T) {
	var samples [runtimeSparklinePoints]uint64
	for i := 0; i < 10; i++ {
		samples[i] = uint64(i + 1)
	}
	out := renderRateBrailleRing(samples, 10, 10, 10)
	if utf8.RuneCountInString(out) != 10 {
		t.Fatalf("expected braille width 10, got %d (%q)", utf8.RuneCountInString(out), out)
	}

	unique := map[rune]struct{}{}
	for _, r := range out {
		unique[r] = struct{}{}
	}
	if len(unique) < 2 {
		t.Fatalf("expected non-flat sparkline with multiple glyphs, got %q", out)
	}
}

func TestBrailleDotMaskAndSetBrailleDot(t *testing.T) {
	if got := brailleDotMask(0, 0); got != 1 {
		t.Fatalf("expected left/top mask 1, got %d", got)
	}
	if got := brailleDotMask(1, 3); got != 128 {
		t.Fatalf("expected right/bottom mask 128, got %d", got)
	}

	cells := make([]uint8, 1)
	setBrailleDot(cells, 0, 0) // left top
	setBrailleDot(cells, 1, 3) // right bottom
	if cells[0] != 129 {
		t.Fatalf("expected combined mask 129, got %d", cells[0])
	}

	// out-of-range calls must be safe no-op
	setBrailleDot(cells, -1, 0)
	setBrailleDot(cells, 3, 0)
}

func TestUpdateConfirm_UpLeftDownRight(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.confirmOpen = true
	m.confirmCursor = 1

	// Up decrements cursor when > 0
	updatedModel, _ := m.updateConfirm(tea.KeyMsg{Type: tea.KeyUp})
	updated := updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 0 {
		t.Fatalf("expected cursor 0 after Up, got %d", updated.confirmCursor)
	}

	// Up at cursor=0 stays at 0
	updatedModel, _ = updated.updateConfirm(tea.KeyMsg{Type: tea.KeyUp})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 0 {
		t.Fatalf("expected cursor to stay at 0 after Up at top, got %d", updated.confirmCursor)
	}

	// Left works same as Up
	m.confirmCursor = 1
	updatedModel, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyLeft})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 0 {
		t.Fatalf("expected cursor 0 after Left, got %d", updated.confirmCursor)
	}

	// Down increments cursor when < 1
	m.confirmCursor = 0
	updatedModel, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyDown})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 1 {
		t.Fatalf("expected cursor 1 after Down, got %d", updated.confirmCursor)
	}

	// Down at cursor=1 stays at 1
	updatedModel, _ = updated.updateConfirm(tea.KeyMsg{Type: tea.KeyDown})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 1 {
		t.Fatalf("expected cursor to stay at 1 after Down at bottom, got %d", updated.confirmCursor)
	}

	// Right works same as Down
	m.confirmCursor = 0
	updatedModel, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyRight})
	updated = updatedModel.(RuntimeDashboard)
	if updated.confirmCursor != 1 {
		t.Fatalf("expected cursor 1 after Right, got %d", updated.confirmCursor)
	}
}

func TestUpdateConfirm_EscClosesConfirm(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.confirmOpen = true
	m.confirmCursor = 1

	updatedModel, _ := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEsc})
	updated := updatedModel.(RuntimeDashboard)
	if updated.confirmOpen {
		t.Fatal("expected confirmOpen=false after Esc")
	}
	if updated.confirmCursor != 0 {
		t.Fatalf("expected confirmCursor reset to 0 after Esc, got %d", updated.confirmCursor)
	}
}

func TestUpdateConfirm_EnterAtCursor0_ClosesConfirm(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.confirmOpen = true
	m.confirmCursor = 0

	updatedModel, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(RuntimeDashboard)
	if updated.confirmOpen {
		t.Fatal("expected confirmOpen=false after Enter at cursor=0 (Stay)")
	}
	if cmd != nil {
		t.Fatal("expected no quit command when selecting Stay")
	}
}

func TestUpdateConfirm_EnterAtCursor1_Reconfigures(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.confirmOpen = true
	m.confirmCursor = 1

	updatedModel, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.reconfigureRequested {
		t.Fatal("expected reconfigureRequested=true after Enter at cursor=1")
	}
	if cmd == nil {
		t.Fatal("expected quit command when confirming reconfigure")
	}
}

func TestUpdateConfirm_CtrlCDuringConfirmExits(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.confirmOpen = true

	updatedModel, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.exitRequested {
		t.Fatal("expected exitRequested=true after ctrl+c during confirm")
	}
	if cmd == nil {
		t.Fatal("expected quit command on ctrl+c during confirm")
	}
}

func TestUpdateLogs_DownKeyNotAtBottom(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	m.logViewport.GotoTop()
	m.logFollow = false

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyDown})
	updated := updatedModel.(RuntimeDashboard)
	if updated.logFollow {
		t.Fatal("expected logFollow=false when not at bottom after Down")
	}
}

func TestUpdateLogs_UpSetFollowFalse(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	m.logFollow = true

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyUp})
	updated := updatedModel.(RuntimeDashboard)
	if updated.logFollow {
		t.Fatal("expected logFollow=false after Up")
	}
}

func TestUpdateLogs_PgDownAtBottomSetsFollow(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	m.logViewport.GotoBottom()

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyPgDown})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.logFollow {
		t.Fatal("expected logFollow=true after PgDown when already at bottom")
	}
}

func TestUpdateLogs_HomeGoesToTop(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyHome})
	updated := updatedModel.(RuntimeDashboard)
	if updated.logFollow {
		t.Fatal("expected logFollow=false after Home")
	}
	if updated.logViewport.YOffset != 0 {
		t.Fatalf("expected viewport offset 0 after Home, got %d", updated.logViewport.YOffset)
	}
}

func TestUpdateLogs_EndGoesToBottom(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	m.logViewport.GotoTop()

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyEnd})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.logFollow {
		t.Fatal("expected logFollow=true after End")
	}
	if !updated.logViewport.AtBottom() {
		t.Fatal("expected viewport at bottom after End")
	}
}

func TestUpdateLogs_SpaceTogglesFollow(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: lines},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	m.logFollow = false

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeySpace})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.logFollow {
		t.Fatal("expected logFollow=true after Space toggle from false")
	}

	updatedModel, _ = updated.updateLogs(tea.KeyMsg{Type: tea.KeySpace})
	updated = updatedModel.(RuntimeDashboard)
	if updated.logFollow {
		t.Fatal("expected logFollow=false after Space toggle from true")
	}
}

func TestRuntimeLogUpdateCmd_PlainFeedFallsBackToTick(t *testing.T) {
	feed := testRuntimeLogFeed{lines: []string{"line"}}
	stop := make(chan struct{})
	cmd := runtimeLogUpdateCmd(context.Background(), feed, stop, 1)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	// The returned command should be a tick cmd (time-based), not a channel wait.
	// We verify it returns a runtimeLogTickMsg eventually.
	msg := cmd()
	if _, ok := msg.(runtimeLogTickMsg); !ok {
		t.Fatalf("expected runtimeLogTickMsg from plain feed fallback, got %T", msg)
	}
}

type testRuntimeChangeFeed struct {
	testRuntimeLogFeed
	changes chan struct{}
}

func (f testRuntimeChangeFeed) Changes() <-chan struct{} {
	return f.changes
}

func TestRuntimeLogUpdateCmd_ChangeFeedNilChanges_FallsBackToTick(t *testing.T) {
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            nil,
	}
	stop := make(chan struct{})
	cmd := runtimeLogUpdateCmd(context.Background(), feed, stop, 1)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(runtimeLogTickMsg); !ok {
		t.Fatalf("expected runtimeLogTickMsg from nil Changes fallback, got %T", msg)
	}
}

func TestZeroBrailleSparkline_WidthEdgeCases(t *testing.T) {
	if got := zeroBrailleSparkline(0); got != "" {
		t.Fatalf("expected empty string for width<=0, got %q", got)
	}
	if got := zeroBrailleSparkline(-5); got != "" {
		t.Fatalf("expected empty string for negative width, got %q", got)
	}

	clamped := zeroBrailleSparkline(runtimeSparklinePoints + 10)
	expected := zeroBrailleSparkline(runtimeSparklinePoints)
	if clamped != expected {
		t.Fatalf("expected width clamped to runtimeSparklinePoints, got %q vs %q", clamped, expected)
	}
	if utf8.RuneCountInString(clamped) != runtimeSparklinePoints {
		t.Fatalf("expected rune count %d, got %d", runtimeSparklinePoints, utf8.RuneCountInString(clamped))
	}
}

func TestSetBrailleDot_EmptyCells(t *testing.T) {
	var cells []uint8
	setBrailleDot(cells, 0, 0) // should not panic
}

func TestSetBrailleDot_NegativeXPixel(t *testing.T) {
	cells := make([]uint8, 2)
	setBrailleDot(cells, -1, 0) // should not panic and not modify cells
	if cells[0] != 0 || cells[1] != 0 {
		t.Fatalf("expected cells unchanged after negative xPixel, got %v", cells)
	}
}

func TestSetBrailleDot_CellIndexOutOfRange(t *testing.T) {
	cells := make([]uint8, 1)
	setBrailleDot(cells, 4, 0) // cellIndex=2 >= len(cells)=1, should be no-op
	if cells[0] != 0 {
		t.Fatalf("expected cell unchanged when cellIndex out of range, got %d", cells[0])
	}
}

func TestSetBrailleDot_YRowClamping(t *testing.T) {
	cells := make([]uint8, 1)

	// yRow < 0 should be clamped to 0
	setBrailleDot(cells, 0, -1)
	expected := brailleDotMask(0, 0) // yRow clamped to 0
	if cells[0] != expected {
		t.Fatalf("expected mask for yRow=0 (%d), got %d", expected, cells[0])
	}

	cells[0] = 0
	// yRow > 3 should be clamped to 3
	setBrailleDot(cells, 0, 5)
	expected = brailleDotMask(0, 3) // yRow clamped to 3
	if cells[0] != expected {
		t.Fatalf("expected mask for yRow=3 (%d), got %d", expected, cells[0])
	}
}

func TestRingSampleAt_EdgeCases(t *testing.T) {
	var samples [runtimeSparklinePoints]uint64
	samples[0] = 42

	// count=0 returns 0
	if got := ringSampleAt(samples, 0, 0, 0); got != 0 {
		t.Fatalf("expected 0 for count=0, got %d", got)
	}

	// pos < 0 returns 0
	if got := ringSampleAt(samples, 1, 1, -1); got != 0 {
		t.Fatalf("expected 0 for pos<0, got %d", got)
	}

	// pos >= count returns 0
	if got := ringSampleAt(samples, 1, 1, 1); got != 0 {
		t.Fatalf("expected 0 for pos>=count, got %d", got)
	}
}

func TestHandleGraphPreferenceChange_FalseToTrue(t *testing.T) {
	m := RuntimeDashboard{}
	m.preferences.ShowDataplaneGraph = true

	m.handleGraphPreferenceChange(false)

	// The false->true transition should call recordTrafficSample,
	// which increments sampleCount.
	if m.sampleCount != 1 {
		t.Fatalf("expected sampleCount=1 after false->true transition, got %d", m.sampleCount)
	}
}

func TestHandleGraphPreferenceChange_TrueToFalse(t *testing.T) {
	m := RuntimeDashboard{}
	m.preferences.ShowDataplaneGraph = false
	m.sampleCount = 5
	m.sampleCursor = 3

	m.handleGraphPreferenceChange(true)

	// The true->false transition should clear samples.
	if m.sampleCount != 0 {
		t.Fatalf("expected sampleCount=0 after true->false transition, got %d", m.sampleCount)
	}
	if m.sampleCursor != 0 {
		t.Fatalf("expected sampleCursor=0 after true->false transition, got %d", m.sampleCursor)
	}
}

func TestHandleGraphPreferenceChange_NoChange(t *testing.T) {
	m := RuntimeDashboard{}
	m.preferences.ShowDataplaneGraph = true
	m.sampleCount = 3

	m.handleGraphPreferenceChange(true)
	// No change: sampleCount should remain as-is.
	if m.sampleCount != 3 {
		t.Fatalf("expected sampleCount unchanged when no transition, got %d", m.sampleCount)
	}
}

func TestEnsureLogsViewport_WhenLogReadyFalse(t *testing.T) {
	m := RuntimeDashboard{
		width:       100,
		height:      30,
		preferences: CurrentUIPreferences(),
	}
	m.logReady = false

	m.ensureLogsViewport()
	if !m.logReady {
		t.Fatal("expected logReady=true after ensureLogsViewport")
	}
	if m.logViewport.Width <= 0 {
		t.Fatalf("expected viewport width > 0, got %d", m.logViewport.Width)
	}
}

func TestEnsureLogsViewport_WhenLogReadyTrue_Resizes(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.width = 80
	m.height = 20
	origWidth := m.logViewport.Width

	m.width = 120
	m.height = 30
	m.ensureLogsViewport()
	if m.logViewport.Width == origWidth {
		t.Fatal("expected viewport width to change after resize")
	}
}

func TestRuntimeDashboard_Update_LogTickMismatchedSeqOnLogsScreen(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: []string{"one", "two"}},
	})
	// Navigate to logs screen.
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})  // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyTab}) // logs
	dash := m2.(RuntimeDashboard)

	// Send a runtimeLogTickMsg with a mismatched seq while on logs screen.
	wrongSeq := dash.logTickSeq + 99
	updatedModel, cmd := dash.Update(runtimeLogTickMsg{seq: wrongSeq})
	if cmd != nil {
		t.Fatal("expected nil cmd for mismatched log tick seq on logs screen")
	}
	_ = updatedModel.(RuntimeDashboard)
}

func TestUpdateLogs_DownKeyAtBottom_SetsFollowTrue(t *testing.T) {
	// Use very few lines so the viewport is already at bottom.
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: []string{"a"}},
	})
	m.width = 120
	m.height = 24
	m.screen = runtimeScreenLogs
	m.refreshLogs()
	// Ensure viewport is at bottom.
	m.logViewport.GotoBottom()
	m.logFollow = false

	updatedModel, _ := m.updateLogs(tea.KeyMsg{Type: tea.KeyDown})
	updated := updatedModel.(RuntimeDashboard)
	if !updated.logFollow {
		t.Fatal("expected logFollow=true when Down key pressed and viewport is at bottom")
	}
}

func TestRuntimeLogUpdateCmd_StopClosedReturnsLogTickMsg(t *testing.T) {
	// Use a change feed with a valid channel so we enter the select branch.
	changes := make(chan struct{}, 1)
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            changes,
	}
	stop := make(chan struct{})
	close(stop) // close immediately

	cmd := runtimeLogUpdateCmd(context.Background(), feed, stop, 42)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	tick, ok := msg.(runtimeLogTickMsg)
	if !ok {
		t.Fatalf("expected runtimeLogTickMsg when stop is closed, got %T", msg)
	}
	// When stop fires, seq should be zero (not the passed-in seq).
	if tick.seq != 0 {
		t.Fatalf("expected seq=0 from stop branch, got %d", tick.seq)
	}
}

func TestRuntimeLogUpdateCmd_ContextCanceled(t *testing.T) {
	changes := make(chan struct{}, 1)
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            changes,
	}
	stop := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := runtimeLogUpdateCmd(ctx, feed, stop, 42)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(runtimeContextDoneMsg); !ok {
		t.Fatalf("expected runtimeContextDoneMsg when context is canceled, got %T", msg)
	}
}

func TestRuntimeLogUpdateCmd_ChangeFeedSignalReturnsMatchingSeq(t *testing.T) {
	changes := make(chan struct{}, 1)
	changes <- struct{}{} // signal immediately
	feed := testRuntimeChangeFeed{
		testRuntimeLogFeed: testRuntimeLogFeed{lines: []string{"line"}},
		changes:            changes,
	}
	stop := make(chan struct{})

	cmd := runtimeLogUpdateCmd(context.Background(), feed, stop, 42)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	msg := cmd()
	tick, ok := msg.(runtimeLogTickMsg)
	if !ok {
		t.Fatalf("expected runtimeLogTickMsg from changes signal, got %T", msg)
	}
	if tick.seq != 42 {
		t.Fatalf("expected seq=42 from changes signal, got %d", tick.seq)
	}
}

func TestBrailleRow_ValueEqualsMaxValue(t *testing.T) {
	// When value == maxValue, level = (100*3)/100 = 3, row = 3-3 = 0.
	if got := brailleRow(100, 100); got != 0 {
		t.Fatalf("expected brailleRow(100, 100) == 0, got %d", got)
	}
	if got := brailleRow(50, 100); got == 0 {
		t.Fatalf("expected brailleRow(50, 100) != 0, got %d", got)
	}
}

func TestRenderRateBrailleRing_WidthZeroDefaults(t *testing.T) {
	var samples [runtimeSparklinePoints]uint64
	for i := 0; i < 5; i++ {
		samples[i] = uint64(i + 1)
	}
	// width=0 should default to min(runtimeSparklinePoints, count) = min(40, 5) = 5.
	out := renderRateBrailleRing(samples, 5, 5, 0)
	if out == "no-data" {
		t.Fatal("expected actual braille output for width=0 with data")
	}
	runeCount := 0
	for range out {
		runeCount++
	}
	if runeCount != 5 {
		t.Fatalf("expected default width of 5 runes, got %d", runeCount)
	}
}
