package bubble_tea

import (
	"errors"
	"os"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_area"

	tea "charm.land/bubbletea/v2"
)

type TextAreaAdapter struct {
	textArea interface {
		Value() string
		Cancelled() bool
	}
	runner programRunner
}

func NewTextAreaAdapter() text_area.TextAreaFactory {
	return newTextAreaAdapterWithRunner(newProgramRunner())
}

func newTextAreaAdapterWithRunner(runner programRunner) *TextAreaAdapter {
	return &TextAreaAdapter{runner: runner}
}

func (t *TextAreaAdapter) NewTextArea(ph string) (text_area.TextArea, error) {
	ta := NewTextArea(ph)
	res, err := t.runner.Run(ta, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout))
	if err != nil {
		return nil, err
	}

	taLike, ok := res.(interface {
		Value() string
		Cancelled() bool
	})
	if !ok {
		return nil, errors.New("unexpected textArea type")
	}
	t.textArea = taLike
	return t, nil
}

func (t *TextAreaAdapter) Value() (string, error) {
	if t.textArea.Cancelled() {
		return "", text_area.ErrCancelled
	}
	return t.textArea.Value(), nil
}
