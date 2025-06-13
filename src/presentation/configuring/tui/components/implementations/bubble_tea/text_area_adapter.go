package bubble_tea

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"tungo/presentation/configuring/tui/components"
)

type TextAreaAdapter struct {
	textArea TextArea
}

func NewTextAreaAdapter() components.TextAreaFactory {
	return &TextAreaAdapter{}
}

func (t *TextAreaAdapter) NewTextArea(placeholder string) (components.TextArea, error) {
	textArea := NewTextArea(placeholder)
	textAreaProgram, textAreaProgramErr := tea.
		NewProgram(textArea, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout)).Run()
	if textAreaProgramErr != nil {
		return nil, textAreaProgramErr
	}

	textAreaResult, ok := textAreaProgram.(*TextArea)
	if !ok {
		return nil, errors.New("unexpected textArea type")
	}

	t.textArea = *textAreaResult

	return t, textAreaProgramErr
}

func (t *TextAreaAdapter) Value() (string, error) {
	return t.textArea.Value(), nil
}
