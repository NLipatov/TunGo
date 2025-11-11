package bubble_tea

import (
	"errors"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

type SelectorAdapter struct {
	selector  Selector
	teaRunner TeaRunner
}

func NewSelectorAdapter() selector.Factory {
	return &SelectorAdapter{
		teaRunner: &defaultTeaRunner{},
	}
}

func NewCustomTeaRunnerSelectorAdapter(teaRunner TeaRunner) selector.Factory {
	return &SelectorAdapter{
		teaRunner: teaRunner,
	}
}

func (s *SelectorAdapter) NewTuiSelector(
	placeholder string,
	options []string,
	foregroundColor, backgroundColor value_objects.Color,
) (selector.Selector, error) {
	newSelector := NewSelector(placeholder, options, NewColorizer(), foregroundColor, backgroundColor)
	selectorProgram, selectorProgramErr := s.teaRunner.Run(newSelector)
	if selectorProgramErr != nil {
		return nil, selectorProgramErr
	}

	selectorResult, ok := selectorProgram.(Selector)
	if !ok {
		return nil, errors.New("invalid selector type")
	}

	s.selector = selectorResult

	return s, nil
}

func (s *SelectorAdapter) SelectOne() (string, error) {
	return s.selector.Choice(), nil
}
