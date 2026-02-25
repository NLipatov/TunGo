package bubble_tea

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testSettings() *uiPreferencesProvider {
	return newDefaultUIPreferencesProvider()
}

func TestFatalErrorModel_View_ContainsMessage(t *testing.T) {
	m := newFatalErrorModel("Something went wrong", testSettings())
	m.width = 100
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Something went wrong") {
		t.Fatalf("expected view to contain message, got %q", view)
	}
}

func TestFatalErrorModel_View_HasTabsLineWithProductLabel(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	m.width = 100
	m.height = 30
	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Fatalf("expected view to contain Error tab, got %q", view)
	}
	if !strings.Contains(view, productLabel()) {
		t.Fatalf("expected view to contain product label %q, got %q", productLabel(), view)
	}
}

func TestFatalErrorModel_View_ContainsANSI(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)
	m := newFatalErrorModel("details", testSettings())
	view := m.View()
	if !containsANSI(view) {
		t.Fatalf("expected view to contain ANSI codes for theming, got %q", view)
	}
}

func TestFatalErrorModel_View_ContainsDismissHint(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	view := m.View()
	if !strings.Contains(view, "Press Enter to exit") {
		t.Fatalf("expected view to contain dismiss hint, got %q", view)
	}
}

func TestFatalErrorModel_Update_EnterQuits(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit command on Enter")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestFatalErrorModel_Update_EscQuits(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected quit command on Esc")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestFatalErrorModel_Update_QKeyQuits(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command on 'q'")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestFatalErrorModel_Update_ArbitraryKeyDoesNotQuit(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("expected no command on arbitrary key, got %v", cmd)
	}
}

func TestFatalErrorModel_Update_WindowSizeUpdates(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(fatalErrorModel)
	if model.width != 120 || model.height != 40 {
		t.Fatalf("expected dimensions 120x40, got %dx%d", model.width, model.height)
	}
}

func TestFatalErrorModel_View_RespectsTheme(t *testing.T) {
	forceANSIColorProfile(t, ansiColorProfileTrueColor)

	themes := []ThemeOption{ThemeLight, ThemeDark, ThemeDarkHighContrast, ThemeDarkMatrix}
	views := make([]string, len(themes))

	s := testSettings()
	for i, theme := range themes {
		p := s.Preferences()
		p.Theme = theme
		s.update(p)
		m := newFatalErrorModel("details", s)
		views[i] = m.View()
	}

	// At least light vs dark should produce different output (different ANSI bg codes)
	if views[0] == views[1] {
		t.Fatal("expected different views for light vs dark theme")
	}
}

func TestFatalErrorModel_Init_ReturnsNil(t *testing.T) {
	m := newFatalErrorModel("details", testSettings())
	cmd := m.Init()
	if cmd != nil {
		t.Fatalf("expected nil Init command, got %v", cmd)
	}
}

func TestNewFatalErrorProgram_ReturnsNonNil(t *testing.T) {
	p := NewFatalErrorProgram("details")
	if p == nil {
		t.Fatal("expected non-nil tea.Program")
	}
}
