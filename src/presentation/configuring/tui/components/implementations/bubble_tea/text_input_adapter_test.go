package bubble_tea

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

type fakeModel struct{}

func (f *fakeModel) Init() tea.Cmd                       { return nil }
func (f *fakeModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f *fakeModel) View() string                        { return "" }

type fakeRunner struct {
	result tea.Model
	err    error
}

func (r *fakeRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return r.result, r.err
}

func TestNewTextInput_Success(t *testing.T) {
	adapter := NewCustomTeaRunnerTextInputAdapter(&fakeRunner{
		result: &TextInput{},
		err:    nil,
	}).(*TextInputAdapter)

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

func TestNewTextInput_RunError(t *testing.T) {
	want := errors.New("boom")
	adapter := NewCustomTeaRunnerTextInputAdapter(&fakeRunner{
		result: nil,
		err:    want,
	}).(*TextInputAdapter)

	_, err := adapter.NewTextInput("pl")
	if !errors.Is(err, want) {
		t.Errorf("expected Run error %v, got %v", want, err)
	}
}

func TestNewTextInput_InvalidFormat(t *testing.T) {
	adapter := NewCustomTeaRunnerTextInputAdapter(&fakeRunner{
		result: &fakeModel{},
		err:    nil,
	}).(*TextInputAdapter)

	_, err := adapter.NewTextInput("pl")
	if err == nil || err.Error() != "invalid textInput format" {
		t.Errorf("expected invalid format error, got %v", err)
	}
}
