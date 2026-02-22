package bubble_tea

import (
	"errors"
	"testing"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_area"

	tea "github.com/charmbracelet/bubbletea"
)

type textAreaAdapterMockTeaRunner struct {
	ret tea.Model
	err error
}

func (m *textAreaAdapterMockTeaRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return m.ret, m.err
}

type textAreaMock struct {
	val       string
	cancelled bool
}

func (f *textAreaMock) Init() tea.Cmd {
	panic("not implemented")
}

func (f *textAreaMock) Update(_ tea.Msg) (tea.Model, tea.Cmd) {
	panic("not implemented")
}

func (f *textAreaMock) View() string {
	panic("not implemented")
}

func (f *textAreaMock) Value() string { return f.val }
func (f *textAreaMock) Cancelled() bool {
	return f.cancelled
}

type dummyModel struct{}

func (d dummyModel) Init() tea.Cmd                         { return nil }
func (d dummyModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d dummyModel) View() string                          { return "" }

func TestTextAreaAdapter_NewTextArea_Success(t *testing.T) {
	fta := &textAreaMock{val: "ok"}
	adapter := newTextAreaAdapterWithRunner(&textAreaAdapterMockTeaRunner{ret: fta})

	sel, err := adapter.NewTextArea("ph")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel != adapter {
		t.Fatalf("selector should be adapter itself")
	}
	got, _ := adapter.Value()
	if got != "ok" {
		t.Fatalf("want %q, got %q", "ok", got)
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

func TestTextAreaAdapter_Value_Empty(t *testing.T) {
	adapter := &TextAreaAdapter{
		textArea: &textAreaMock{val: ""},
	}
	if v, _ := adapter.Value(); v != "" {
		t.Fatalf("want empty string, got %q", v)
	}
}

func TestTextAreaAdapter_Value_Cancelled(t *testing.T) {
	adapter := &TextAreaAdapter{
		textArea: &textAreaMock{val: "", cancelled: true},
	}
	_, err := adapter.Value()
	if !errors.Is(err, text_area.ErrCancelled) {
		t.Fatalf("want ErrCancelled, got %v", err)
	}
}
