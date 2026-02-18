package bubble_tea

import (
	"errors"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
)

type TextInputAdapter struct {
	input     TextInput
	teaRunner TeaRunner
}

func NewTextInputAdapter() text_input.TextInputFactory {
	return &TextInputAdapter{
		teaRunner: &defaultTeaRunner{},
	}
}

func NewCustomTeaRunnerTextInputAdapter(teaRunner TeaRunner) text_input.TextInputFactory {
	return &TextInputAdapter{
		teaRunner: teaRunner,
	}
}

func (t *TextInputAdapter) NewTextInput(placeholder string) (text_input.TextInput, error) {
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
	if t.input.Cancelled() {
		return "", text_input.ErrCancelled
	}
	return t.input.Value(), nil
}
