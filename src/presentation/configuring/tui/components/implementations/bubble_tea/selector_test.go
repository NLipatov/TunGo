package bubble_tea

import (
	"strings"
	"testing"

	"tungo/presentation/configuring/tui/components/domain/value_objects"

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
		t.Errorf("expected Init to return nil cmd")
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
	if _, ok := updatedModel.(Selector); !ok {
		t.Fatal("Update did not return Selector")
	}
	if cmd == nil {
		t.Error("expected quit command on 'q'")
	}
}

func TestSelector_View_Normal_HighlightsCursor(t *testing.T) {
	sel, colorizer := newTestSelector("client mode", "server mode")
	sel.cursor = 0
	view := sel.View()

	if !strings.Contains(view, sel.placeholder) {
		t.Errorf("view should contain placeholder %q", sel.placeholder)
	}
	if colorizer.calls == 0 {
		t.Fatal("expected colorizer to be called for highlighted line")
	}
	if !strings.Contains(view, "[["+colorizer.lastS+"]]") {
		t.Errorf("highlight marker not found in view")
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

func TestSelector_SettingsToggleFooter(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeAuto
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeAuto
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab})            // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // language row
	m3, _ := m2.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	m4, _ := m3.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // footer row
	_, _ = m4.(Selector).Update(tea.KeyMsg{Type: tea.KeyRight})  // toggle

	if CurrentUIPreferences().ShowFooter {
		t.Fatalf("expected ShowFooter to be toggled off")
	}
}

func TestSelector_SettingsToggleStatsUnits(t *testing.T) {
	UpdateUIPreferences(func(p *UIPreferences) {
		p.Theme = ThemeAuto
		p.Language = "en"
		p.StatsUnits = StatsUnitsBiBytes
		p.ShowFooter = true
	})
	t.Cleanup(func() {
		UpdateUIPreferences(func(p *UIPreferences) {
			p.Theme = ThemeAuto
			p.Language = "en"
			p.StatsUnits = StatsUnitsBiBytes
			p.ShowFooter = true
		})
	})

	sel, _ := newTestSelector("Main title", "a", "b")
	m1, _ := sel.Update(tea.KeyMsg{Type: tea.KeyTab})            // settings
	m2, _ := m1.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // language row
	m3, _ := m2.(Selector).Update(tea.KeyMsg{Type: tea.KeyDown}) // stats units row
	_, _ = m3.(Selector).Update(tea.KeyMsg{Type: tea.KeyRight})  // toggle

	if CurrentUIPreferences().StatsUnits != StatsUnitsBytes {
		t.Fatalf("expected StatsUnits to be toggled to bytes")
	}
}
