package bubble_tea

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

	if !strings.Contains(view, "Status: connected") {
		t.Fatalf("expected dataplane screen after third Tab, got view: %q", view)
	}
}

func TestRuntimeDashboard_TogglesFooterInSettings(t *testing.T) {
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

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})                      // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // language row
	m3, _ := m2.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m4, _ := m3.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // footer row
	_, _ = m4.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyRight})  // toggle

	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected ShowFooter to be toggled off")
	}
}

func TestRuntimeDashboard_TogglesStatsUnitsInSettings(t *testing.T) {
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

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})                      // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // language row
	m3, _ := m2.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	_, _ = m3.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyRight})  // toggle

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

func TestRunRuntimeDashboard_FinalModelTypeAndQuitFlag(t *testing.T) {
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
				m.quitRequested = true
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
	if cmd := runtimeTickCmd(); cmd == nil {
		t.Fatal("expected runtimeTickCmd command")
	}
	if cmd := runtimeLogTickCmd(); cmd == nil {
		t.Fatal("expected runtimeLogTickCmd command")
	}
}

func TestRuntimeDashboard_Update_WindowAndContextDoneAndQuit(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		LogFeed: testRuntimeLogFeed{lines: []string{"one", "two"}},
	})
	updatedModel, cmd := m.Update(runtimeTickMsg{})
	if cmd == nil {
		t.Fatal("expected follow-up tick cmd on runtimeTickMsg")
	}
	updated := updatedModel.(RuntimeDashboard)

	updatedModel, cmd = updated.Update(runtimeLogTickMsg{})
	if cmd == nil {
		t.Fatal("expected follow-up log tick cmd on runtimeLogTickMsg")
	}
	updated = updatedModel.(RuntimeDashboard)

	updatedModel, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated = updatedModel.(RuntimeDashboard)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("unexpected size: %dx%d", updated.width, updated.height)
	}
	if len(updated.logLines) == 0 {
		t.Fatal("expected logs refreshed on window size message")
	}

	updatedModel, cmd = updated.Update(runtimeContextDoneMsg{})
	if cmd == nil {
		t.Fatal("expected quit cmd on runtimeContextDoneMsg")
	}
	updated = updatedModel.(RuntimeDashboard)

	updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit cmd on q")
	}
	if !updatedModel.(RuntimeDashboard).quitRequested {
		t.Fatal("expected quitRequested flag on q")
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

func TestRuntimeDashboard_SettingsNavigationAndMutation(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeLight
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
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

	// Language row: Select keeps EN.
	m.settingsCursor = settingsLanguageRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().Language != "en" {
		t.Fatalf("expected language en, got %q", CurrentUIPreferences().Language)
	}

	// Stats units row: Enter toggles.
	m.settingsCursor = settingsStatsUnitsRow
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if CurrentUIPreferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected stats units bytes, got %q", CurrentUIPreferences().StatsUnits)
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
		p.ShowFooter = false
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.ShowFooter = true
		})
	})

	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{
		Mode: RuntimeDashboardServer,
	})
	m.width = 120
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Server runtime - Workers are running") {
		t.Fatalf("expected server subtitle, got %q", view)
	}
	if !strings.Contains(view, "Status: running") {
		t.Fatalf("expected running status in main view, got %q", view)
	}
}

func TestRuntimeDashboard_RefreshLogsNilFeed(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m.logLines = []string{"stale"}
	m.refreshLogs()
	if m.logLines != nil {
		t.Fatalf("expected nil log lines when feed is absent, got %v", m.logLines)
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
	if _, ok := runtimeLogTickCmd()().(runtimeLogTickMsg); !ok {
		t.Fatal("expected runtimeLogTickMsg")
	}
	if _, ok := runtimeTickCmd()().(runtimeTickMsg); !ok {
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
