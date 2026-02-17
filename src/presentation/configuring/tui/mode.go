package tui

import (
	"fmt"
	"tungo/domain/mode"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

type AppMode struct {
	selectorFactory selector.Factory
}

func NewAppMode(selectorFactory selector.Factory) AppMode {
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
