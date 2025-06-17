package bubble_tea

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type selectorAdapterMockModel struct{}

func (f selectorAdapterMockModel) Init() tea.Cmd {
	return nil
}

func (f selectorAdapterMockModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) {
	return f, nil
}

func (f selectorAdapterMockModel) View() string {
	return ""
}

type mockTeaRunner struct {
	returnModel tea.Model
	returnErr   error
}

func (m *mockTeaRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return m.returnModel, m.returnErr
}

func TestSelectorAdapter_NewTuiSelector_Success(t *testing.T) {
	mockSel := NewSelector("placeholder", []string{"opt1", "opt2"})

	mockRunner := &mockTeaRunner{returnModel: mockSel, returnErr: nil}
	adapter := NewCustomTeaRunnerSelectorAdapter(mockRunner).(*SelectorAdapter)

	sel, err := adapter.NewTuiSelector("placeholder", []string{"opt1", "opt2"})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sel == nil {
		t.Fatal("expected non-nil selector")
	}
	if sel != adapter {
		t.Fatal("expected returned selector to be adapter itself")
	}
	if adapter.selector.Choice() != "" {
		t.Fatalf("expected empty choice, got %q", adapter.selector.Choice())
	}
}

func TestSelectorAdapter_NewTuiSelector_RunError(t *testing.T) {
	mockRunner := &mockTeaRunner{returnModel: nil, returnErr: errors.New("run error")}
	adapter := NewCustomTeaRunnerSelectorAdapter(mockRunner).(*SelectorAdapter)

	sel, err := adapter.NewTuiSelector("placeholder", []string{"opt1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if sel != nil {
		t.Fatal("expected nil selector on error")
	}
}

func TestSelectorAdapter_NewTuiSelector_InvalidType(t *testing.T) {
	mockRunner := &mockTeaRunner{returnModel: selectorAdapterMockModel{}, returnErr: nil} // invalid type
	adapter := NewCustomTeaRunnerSelectorAdapter(mockRunner).(*SelectorAdapter)

	sel, err := adapter.NewTuiSelector("placeholder", []string{"opt1"})
	if err == nil {
		t.Fatal("expected error on invalid type, got nil")
	}
	if sel != nil {
		t.Fatal("expected nil selector on invalid type error")
	}
}

func TestSelectorAdapter_SelectOne(t *testing.T) {
	adapter := &SelectorAdapter{
		selector: Selector{
			placeholder: "ph",
			options:     []string{"opt1", "opt2"},
			choice:      "opt2",
		},
	}

	choice, err := adapter.SelectOne()
	if err != nil {
		t.Fatalf("unexpected error from SelectOne: %v", err)
	}
	if choice != "opt2" {
		t.Fatalf("expected choice %q, got %q", "opt2", choice)
	}
}
