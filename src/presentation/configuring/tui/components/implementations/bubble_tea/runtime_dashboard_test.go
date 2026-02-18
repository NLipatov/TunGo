package bubble_tea

import (
	"context"
	"strings"
	"testing"

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

func TestRuntimeDashboard_TabSwitchesToLogs(t *testing.T) {
	m := NewRuntimeDashboard(context.Background(), RuntimeDashboardOptions{})
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // settings
	m2, _ := m1.(RuntimeDashboard).Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m2.(RuntimeDashboard).View()

	if !strings.Contains(view, "Logs") {
		t.Fatalf("expected logs screen after second Tab, got view: %q", view)
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
