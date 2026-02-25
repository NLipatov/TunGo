package bubble_tea

import (
	"errors"
	"testing"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_input"

	"charm.land/bubbletea/v2"
)

type TextInputAdapterMockModel struct{}

func (f *TextInputAdapterMockModel) Init() tea.Cmd                       { return nil }
func (f *TextInputAdapterMockModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f *TextInputAdapterMockModel) View() tea.View                       { return tea.NewView("") }

type textInputAdapterMockRunner struct {
	result tea.Model
	err    error
}

func (r *textInputAdapterMockRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return r.result, r.err
}

func TestNewTextInput_Success(t *testing.T) {
	adapter := newTextInputAdapterWithRunner(&textInputAdapterMockRunner{
		result: &TextInput{},
		err:    nil,
	})

	ti, err := adapter.NewTextInput("placeholder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, err := ti.Value()
	if err != nil {
		t.Fatalf("Value returned error: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty value, got %q", v)
	}
}

func TestNewTextInputAdapter_DefaultRunnerConstructs(t *testing.T) {
	f := NewTextInputAdapter()
	if f == nil {
		t.Fatal("expected non-nil factory from NewTextInputAdapter")
	}
}

func TestNewTextInput_RunError(t *testing.T) {
	want := errors.New("boom")
	adapter := newTextInputAdapterWithRunner(&textInputAdapterMockRunner{
		result: nil,
		err:    want,
	})

	_, err := adapter.NewTextInput("pl")
	if !errors.Is(err, want) {
		t.Errorf("expected Run error %v, got %v", want, err)
	}
}

func TestNewTextInput_InvalidFormat(t *testing.T) {
	adapter := newTextInputAdapterWithRunner(&textInputAdapterMockRunner{
		result: &TextInputAdapterMockModel{},
		err:    nil,
	})

	_, err := adapter.NewTextInput("pl")
	if err == nil || err.Error() != "invalid textInput format" {
		t.Errorf("expected invalid format error, got %v", err)
	}
}

func TestTextInputAdapter_Value_Cancelled(t *testing.T) {
	adapter := &TextInputAdapter{
		input: TextInput{cancelled: true},
	}
	_, err := adapter.Value()
	if !errors.Is(err, text_input.ErrCancelled) {
		t.Fatalf("expected ErrCancelled, got %v", err)
	}
}
