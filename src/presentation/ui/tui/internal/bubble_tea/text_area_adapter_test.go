package bubble_tea

import (
	"errors"
	"testing"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_area"

	tea "charm.land/bubbletea/v2"
)

type textAreaAdapterMockTeaRunner struct {
	ret tea.Model
	err error
}

func (m *textAreaAdapterMockTeaRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return m.ret, m.err
}

type dummyModel struct{}

func (d dummyModel) Init() tea.Cmd                         { return nil }
func (d dummyModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d dummyModel) View() tea.View                        { return tea.NewView("") }

func TestTextAreaAdapter_NewTextArea_Success(t *testing.T) {
	ta := NewTextArea("ph")
	adapter := newTextAreaAdapterWithRunner(&textAreaAdapterMockTeaRunner{ret: ta})

	sel, err := adapter.NewTextArea("ph")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel != adapter {
		t.Fatalf("returned value should be adapter itself")
	}
	if adapter.textArea != ta {
		t.Fatal("expected stored textArea to be the one returned by runner")
	}
}

func TestTextAreaAdapter_NewTextAreaAdapter_DefaultRunnerConstructs(t *testing.T) {
	f := NewTextAreaAdapter()
	if f == nil {
		t.Fatal("expected non-nil factory from NewTextAreaAdapter")
	}
}

func TestTextAreaAdapter_NewTextArea_RunError(t *testing.T) {
	adapter := newTextAreaAdapterWithRunner(
		&textAreaAdapterMockTeaRunner{ret: nil, err: errors.New("bang")},
	)

	if sel, err := adapter.NewTextArea("ph"); err == nil || sel != nil {
		t.Fatalf("expected failure from runner")
	}
}

func TestTextAreaAdapter_NewTextArea_InvalidType(t *testing.T) {
	adapter := newTextAreaAdapterWithRunner(
		&textAreaAdapterMockTeaRunner{ret: dummyModel{}, err: nil},
	)

	if sel, err := adapter.NewTextArea("ph"); err == nil || sel != nil {
		t.Fatalf("expected type-assertion error")
	}
}

func TestTextAreaAdapter_Value_NotCancelled(t *testing.T) {
	ta := NewTextArea("ph")
	adapter := &TextAreaAdapter{textArea: ta}
	v, err := adapter.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "" {
		t.Fatalf("want empty string, got %q", v)
	}
}

func TestTextAreaAdapter_Value_Cancelled(t *testing.T) {
	ta := NewTextArea("ph")
	ta.cancelled = true
	adapter := &TextAreaAdapter{textArea: ta}
	_, err := adapter.Value()
	if !errors.Is(err, text_area.ErrCancelled) {
		t.Fatalf("want ErrCancelled, got %v", err)
	}
}
