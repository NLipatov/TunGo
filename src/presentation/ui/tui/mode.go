package tui

import (
	"errors"
	"fmt"
	"tungo/domain/command"
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

func (p *AppMode) Mode() (command.Command, error) {
	clientMode := "client"
	serverMode := "server"
	tuiSelector, selectorErr := p.selectorFactory.NewTuiSelector(
		"Select mode",
		[]string{clientMode, serverMode},
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if selectorErr != nil {
		return command.Unknown, selectorErr
	}

	selectedOption, selectOneErr := tuiSelector.SelectOne()
	if selectOneErr != nil {
		if errors.Is(selectOneErr, selectorContract.ErrNavigateBack) || errors.Is(selectOneErr, selectorContract.ErrUserExit) {
			return command.Unknown, ErrUserExit
		}
		return command.Unknown, selectOneErr
	}
	switch selectedOption {
	case clientMode:
		return command.StartClient, nil
	case serverMode:
		return command.StartServer, nil
	default:
		return command.Unknown, fmt.Errorf("invalid command argument")
	}
}
