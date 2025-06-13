package components

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"tungo/presentation/configuring/tui"
)

type TextInputAdapter struct {
	input TextInput
}

func NewTextInputAdapter() tui.TextInputFactory {
	return &TextInputAdapter{}
}

func (t *TextInputAdapter) NewTextInput(placeholder string) (tui.TextInput, error) {
	textInput := NewTextInput(placeholder)
	textInputProgram, textInputProgramErr := tea.NewProgram(textInput).Run()
	if textInputProgramErr != nil {
		return nil, textInputProgramErr
	}

	textInputResult, textInputResulOk := textInputProgram.(*TextInput)
	if !textInputResulOk {
		return nil, errors.New("invalid textInput format")
	}
	t.input = *textInputResult

	return t, nil
}

func (t *TextInputAdapter) Value() (string, error) {
	return t.input.Value(), nil
}
