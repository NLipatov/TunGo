package bubble_tea

import (
	"errors"
	"tungo/presentation/configuring/tui/components"
)

type TextInputAdapter struct {
	input     TextInput
	teaRunner TeaRunner
}

func NewTextInputAdapter() components.TextInputFactory {
	return &TextInputAdapter{
		teaRunner: &defaultTeaRunner{},
	}
}

func NewCustomTeaRunnerTextInputAdapter(teaRunner TeaRunner) components.TextInputFactory {
	return &TextInputAdapter{
		teaRunner: teaRunner,
	}
}

func (t *TextInputAdapter) NewTextInput(placeholder string) (components.TextInput, error) {
	textInput := NewTextInput(placeholder)
	textInputProgram, textInputProgramErr := t.teaRunner.Run(textInput)
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
