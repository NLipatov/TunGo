package bubble_tea

import (
	"errors"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_input"
)

type TextInputAdapter struct {
	input  TextInput
	runner programRunner
}

func NewTextInputAdapter() text_input.TextInputFactory {
	return newTextInputAdapterWithRunner(newProgramRunner())
}

func newTextInputAdapterWithRunner(runner programRunner) *TextInputAdapter {
	return &TextInputAdapter{runner: runner}
}

func (t *TextInputAdapter) NewTextInput(placeholder string) (text_input.TextInput, error) {
	textInput := NewTextInput(placeholder)
	textInputProgram, textInputProgramErr := t.runner.Run(textInput)
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
	if t.input.Cancelled() {
		return "", text_input.ErrCancelled
	}
	return t.input.Value(), nil
}
