package bubble_tea

import (
	"errors"
	"tungo/presentation/configuring/tui/components"
)

type SelectorAdapter struct {
	selector  Selector
	teaRunner TeaRunner
}

func NewSelectorAdapter() components.SelectorFactory {
	return &SelectorAdapter{
		teaRunner: &defaultTeaRunner{},
	}
}

func NewCustomTeaRunnerSelectorAdapter(teaRunner TeaRunner) components.SelectorFactory {
	return &SelectorAdapter{
		teaRunner: teaRunner,
	}
}

func (s *SelectorAdapter) NewTuiSelector(placeholder string, options []string) (components.Selector, error) {
	selector := NewSelector(placeholder, options)
	selectorProgram, selectorProgramErr := s.teaRunner.Run(selector)
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
