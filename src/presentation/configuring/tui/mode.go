package tui

import (
	"fmt"
	"tungo/domain/mode"
	"tungo/presentation/configuring/tui/components"
)

type AppMode struct {
	selectorFactory components.SelectorFactory
}

func NewAppMode(selectorFactory components.SelectorFactory) AppMode {
	return AppMode{
		selectorFactory: selectorFactory,
	}
}

func (p *AppMode) Mode() (mode.Mode, error) {
	clientMode := "client"
	serverMode := "server"
	selector, selectorErr := p.selectorFactory.NewTuiSelector(
		"Mode selection:",
		[]string{clientMode, serverMode},
		components.NewDefaultColor(),
		components.NewTransparentColor(),
	)
	if selectorErr != nil {
		return mode.Unknown, selectorErr
	}

	selectedOption, selectOneErr := selector.SelectOne()
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
