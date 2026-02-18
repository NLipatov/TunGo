package bubble_tea

import (
	"errors"
	"os"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"

	tea "github.com/charmbracelet/bubbletea"
)

type TextAreaAdapter struct {
	textArea interface {
		Value() string
		Cancelled() bool
	}
	teaRunner TeaRunner
}

func NewTextAreaAdapter() text_area.TextAreaFactory {
	return &TextAreaAdapter{teaRunner: &defaultTeaRunner{}}
}

func NewCustomTeaRunnerTextAreaAdapter(teaRunner TeaRunner) text_area.TextAreaFactory {
	return &TextAreaAdapter{teaRunner: teaRunner}
}

func (t *TextAreaAdapter) NewTextArea(ph string) (text_area.TextArea, error) {
	ta := NewTextArea(ph)
	res, err := t.teaRunner.Run(ta, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout))
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
