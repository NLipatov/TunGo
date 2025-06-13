package components

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"tungo/presentation/configuring/tui"
)

type SelectorAdapter struct {
	selector Selector
}

func NewSelectorAdapter() tui.SelectorFactory {
	return &SelectorAdapter{}
}

func (s *SelectorAdapter) NewTuiSelector(placeholder string, options []string) (tui.Selector, error) {
	selector := NewSelector(placeholder, options)
	selectorProgram, selectorProgramErr := tea.NewProgram(selector).Run()
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
