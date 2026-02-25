package bubble_tea

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewTextArea(t *testing.T) {
	ta := NewTextArea("Type here...")
	if ta == nil {
		t.Fatal("Expected non-nil TextArea")
	}
	if ta.ta.Placeholder != "Type here..." {
		t.Errorf("Expected placeholder %q, got %q", "Type here...", ta.ta.Placeholder)
	}
}

func TestTextArea_Value(t *testing.T) {
	ta := NewTextArea("Type here...")
	if ta.Value() != "" {
		t.Errorf("Expected initial value to be empty, got %q", ta.Value())
	}
}

func TestTextArea_Init(t *testing.T) {
	ta := NewTextArea("Type here...")
	cmd := ta.Init()
	if cmd == nil {
		t.Error("Expected non-nil command from Init")
	}
}

func TestTextArea_UpdateCtrlD_SubmitsAndQuits(t *testing.T) {
	ta := NewTextArea("Type here...")
	msg := tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	model, cmd := ta.Update(msg)

	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	if cmd == nil {
		t.Error("Expected a non-nil quit command on ctrl+d")
	}
	if updatedTA.Value() != "" {
		t.Errorf("Expected value to remain empty, got %q", updatedTA.Value())
	}
}

func TestTextArea_EnterDoesNotSubmit(t *testing.T) {
	ta := NewTextArea("Type here...")
	model, _ := ta.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updatedTA := model.(*TextArea)
	if updatedTA.done {
		t.Error("Enter should not submit textarea (should insert newline)")
	}
}

func TestTextArea_UpdateEsc_CancelsAndQuits(t *testing.T) {
	ta := NewTextArea("Type here...")
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := ta.Update(msg)

	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	if cmd == nil {
		t.Error("Expected a non-nil quit command on esc")
	}
	if !updatedTA.Cancelled() {
		t.Fatal("expected cancelled state after esc")
	}
}

func TestTextArea_UpdateOther(t *testing.T) {
	ta := NewTextArea("Type here...")
	initialValue := ta.Value()
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, cmd := ta.Update(msg)
	_ = cmd // command isn't checked here

	updatedTA, ok := model.(*TextArea)
	if !ok {
		t.Fatal("Expected model to be *TextArea")
	}
	updatedValue := updatedTA.Value()
	if initialValue == updatedValue {
		t.Error("Expected the value to update after key press")
	}
}

func TestTextArea_View(t *testing.T) {
	ta := NewTextArea("Type here...")
	view := ta.View().Content
	if !strings.Contains(view, ta.ta.Placeholder) {
		t.Errorf("Expected view to contain placeholder %q, got %q", ta.ta.Placeholder, view)
	}
}

func TestTextArea_View_WhenDone_Empty(t *testing.T) {
	ta := NewTextArea("Type here...")
	_, _ = ta.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	if view := ta.View().Content; view != "" {
		t.Fatalf("expected empty view when done, got %q", view)
	}
}

func TestTextArea_View_ShowsLineCountForMultilineValue(t *testing.T) {
	ta := NewTextArea("Type here...")
	ta.ta.SetValue("one\ntwo")
	view := ta.View().Content
	if !strings.Contains(view, "Lines: 2") {
		t.Fatalf("expected line count in view, got %q", view)
	}
}

func TestTextArea_UpdateWindowSize_ClampsToCardContentWidth(t *testing.T) {
	ta := NewTextArea("Type here...")
	_, _ = ta.Update(tea.WindowSizeMsg{Width: 220, Height: 40})

	maxAllowed := contentWidthForTerminal(220) - resolveUIStyles(ta.settings.Preferences()).inputFrame.GetHorizontalFrameSize()
	if ta.ta.Width() > maxAllowed {
		t.Fatalf("expected width <= %d, got %d", maxAllowed, ta.ta.Width())
	}
	if ta.ta.Width() > 80 {
		t.Fatalf("expected width to stay stable and not exceed 80, got %d", ta.ta.Width())
	}
	if ta.ta.Width() < 1 {
		t.Fatalf("expected positive width, got %d", ta.ta.Width())
	}
}

func TestTextArea_InputContainerWidth_FallbackToTextAreaWidth(t *testing.T) {
	ta := NewTextArea("Type here...")
	ta.width = 0
	ta.ta.SetWidth(17)

	got := ta.inputContainerWidth()
	want := maxInt(1, ta.ta.Width()+resolveUIStyles(ta.settings.Preferences()).inputFrame.GetHorizontalFrameSize())
	if got != want {
		t.Fatalf("expected fallback width %d, got %d", want, got)
	}
}

func TestTextArea_InputContainerWidth_UsesTerminalWidthWhenKnown(t *testing.T) {
	ta := NewTextArea("Type here...")
	ta.width = 120

	got := ta.inputContainerWidth()
	want := maxInt(1, contentWidthForTerminal(120))
	if got != want {
		t.Fatalf("expected width from terminal content %d, got %d", want, got)
	}
}
