package tui

import (
	"errors"
	"fmt"
	"tungo/domain/mode"
	selectorContract "tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

type AppMode struct {
	selectorFactory selectorContract.Factory
}

func NewAppMode(selectorFactory selectorContract.Factory) AppMode {
	return AppMode{
		selectorFactory: selectorFactory,
	}
}

func (p *AppMode) Mode() (mode.Mode, error) {
	clientMode := "client"
	serverMode := "server"
	tuiSelector, selectorErr := p.selectorFactory.NewTuiSelector(
		"Select mode",
		[]string{clientMode, serverMode},
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if selectorErr != nil {
		return mode.Unknown, selectorErr
	}

	selectedOption, selectOneErr := tuiSelector.SelectOne()
	if selectOneErr != nil {
		if errors.Is(selectOneErr, selectorContract.ErrNavigateBack) || errors.Is(selectOneErr, selectorContract.ErrUserExit) {
			return mode.Unknown, ErrUserExit
		}
		return mode.Unknown, selectOneErr
	}
	switch selectedOption {
	case clientMode:
		return mode.Client, nil
	case serverMode:
		return mode.Server, nil
	default:
		return mode.Unknown, fmt.Errorf("invalid mode argument")
	}
}
