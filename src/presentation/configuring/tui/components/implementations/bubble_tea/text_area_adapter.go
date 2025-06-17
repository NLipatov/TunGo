package bubble_tea

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"tungo/presentation/configuring/tui/components"
)

type TextAreaAdapter struct {
	textArea  interface{ Value() string }
	teaRunner TeaRunner
}

func NewTextAreaAdapter() components.TextAreaFactory {
	return &TextAreaAdapter{teaRunner: &defaultTeaRunner{}}
}

func NewCustomTeaRunnerTextAreaAdapter(teaRunner TeaRunner) components.TextAreaFactory {
	return &TextAreaAdapter{teaRunner: teaRunner}
}

func (t *TextAreaAdapter) NewTextArea(ph string) (components.TextArea, error) {
	ta := NewTextArea(ph)
	res, err := t.teaRunner.Run(ta, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout))
	if err != nil {
		return nil, err
	}

	taLike, ok := res.(interface{ Value() string })
	if !ok {
		return nil, errors.New("unexpected textArea type")
	}
	t.textArea = taLike
	return t, nil
}

func (t *TextAreaAdapter) Value() (string, error) {
	return t.textArea.Value(), nil
}
