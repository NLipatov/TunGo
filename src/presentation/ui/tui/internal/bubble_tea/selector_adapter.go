package bubble_tea

import (
	"errors"
	selectorContract "tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

type SelectorAdapter struct {
	selector Selector
	runner   programRunner
	settings *uiPreferencesProvider
}

func NewSelectorAdapter() selectorContract.Factory {
	return newSelectorAdapterWithRunner(newProgramRunner(), loadUISettingsFromDisk())
}

func newSelectorAdapterWithRunner(runner programRunner, settings *uiPreferencesProvider) *SelectorAdapter {
	return &SelectorAdapter{runner: runner, settings: settings}
}

func (s *SelectorAdapter) NewTuiSelector(
	placeholder string,
	options []string,
	foregroundColor, backgroundColor value_objects.Color,
) (selectorContract.Selector, error) {
	newSelector := NewSelector(placeholder, options, NewColorizer(), foregroundColor, backgroundColor, s.settings)
	selectorProgram, selectorProgramErr := s.runner.Run(newSelector)
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
	if s.selector.QuitRequested() {
		return "", selectorContract.ErrUserExit
	}
	if s.selector.BackRequested() {
		return "", selectorContract.ErrNavigateBack
	}
	return s.selector.Choice(), nil
}
