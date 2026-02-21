package bubble_tea

import (
	"errors"
	"testing"

	selectorContract "tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/value_objects"

	tea "github.com/charmbracelet/bubbletea"
)

type selectorAdapterMockModel struct{}

func (f selectorAdapterMockModel) Init() tea.Cmd                         { return nil }
func (f selectorAdapterMockModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f selectorAdapterMockModel) View() string                          { return "" }

type selectorAdapterMockTeaRunner struct {
	returnModel tea.Model
	returnErr   error
}

func (m *selectorAdapterMockTeaRunner) Run(_ tea.Model, _ ...tea.ProgramOption) (tea.Model, error) {
	return m.returnModel, m.returnErr
}

func TestSelectorAdapter_NewSelectorAdapter_DefaultRunnerConstructs(t *testing.T) {
	f := NewSelectorAdapter()
	if f == nil {
		t.Fatal("expected non-nil factory from NewSelectorAdapter")
	}
}

func TestSelectorAdapter_NewTuiSelector_Success(t *testing.T) {
	mockSel := NewSelector(
		"placeholder",
		[]string{"opt1", "opt2"},
		NewColorizer(),
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)

	mockRunner := &selectorAdapterMockTeaRunner{returnModel: mockSel, returnErr: nil}
	adapter := newSelectorAdapterWithRunner(mockRunner)

	sel, err := adapter.NewTuiSelector(
		"placeholder",
		[]string{"opt1", "opt2"},
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
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
	mockRunner := &selectorAdapterMockTeaRunner{
		returnModel: nil,
		returnErr:   errors.New("run error"),
	}
	adapter := newSelectorAdapterWithRunner(mockRunner)

	sel, err := adapter.NewTuiSelector(
		"placeholder",
		[]string{"opt1"},
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if sel != nil {
		t.Fatal("expected nil selector on error")
	}
}

func TestSelectorAdapter_NewTuiSelector_InvalidType(t *testing.T) {
	mockRunner := &selectorAdapterMockTeaRunner{
		returnModel: selectorAdapterMockModel{},
		returnErr:   nil,
	}
	adapter := newSelectorAdapterWithRunner(mockRunner)

	sel, err := adapter.NewTuiSelector(
		"placeholder",
		[]string{"opt1"},
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
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

func TestSelectorAdapter_SelectOne_BackRequested(t *testing.T) {
	adapter := &SelectorAdapter{
		selector: Selector{
			backRequested: true,
		},
	}

	_, err := adapter.SelectOne()
	if !errors.Is(err, selectorContract.ErrNavigateBack) {
		t.Fatalf("expected selector.ErrNavigateBack, got %v", err)
	}
}

func TestSelectorAdapter_SelectOne_QuitRequested(t *testing.T) {
	adapter := &SelectorAdapter{
		selector: Selector{
			quitRequested: true,
		},
	}

	_, err := adapter.SelectOne()
	if !errors.Is(err, selectorContract.ErrUserExit) {
		t.Fatalf("expected selector.ErrUserExit, got %v", err)
	}
}
