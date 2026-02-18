package tui

import (
	"errors"
	"fmt"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
	"tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type Configurator struct {
	appMode            AppMode
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
}

type configuratorState int

const (
	configuratorStateModeSelect configuratorState = iota
	configuratorStateClient
	configuratorStateServer
)

func NewConfigurator(
	observer clientConfiguration.Observer,
	selector clientConfiguration.Selector,
	creator clientConfiguration.Creator,
	deleter clientConfiguration.Deleter,
	serverConfigurationManager server.ConfigurationManager,
	selectorFactory selector.Factory,
	textInputFactory text_input.TextInputFactory,
	textAreaFactory text_area.TextAreaFactory,
) *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(
			observer,
			selector,
			deleter,
			creator,
			selectorFactory,
			textInputFactory,
			textAreaFactory,
			clientConfiguration.NewManager(),
		),
		serverConfigurator: newServerConfigurator(serverConfigurationManager, selectorFactory),
		appMode:            NewAppMode(selectorFactory),
	}
}

func NewDefaultConfigurator(serverConfigurationManager server.ConfigurationManager) *Configurator {
	clientConfResolver := clientConfiguration.NewDefaultResolver()
	return NewConfigurator(
		clientConfiguration.NewDefaultObserver(clientConfResolver),
		clientConfiguration.NewDefaultSelector(clientConfResolver),
		clientConfiguration.NewDefaultCreator(clientConfResolver),
		clientConfiguration.NewDefaultDeleter(clientConfResolver),
		serverConfigurationManager,
		bubble_tea.NewSelectorAdapter(),
		bubble_tea.NewTextInputAdapter(),
		bubble_tea.NewTextAreaAdapter(),
	)
}

func (p *Configurator) Configure() (mode.Mode, error) {
	state := configuratorStateModeSelect
	selectedMode := mode.Unknown
	for {
		switch state {
		case configuratorStateModeSelect:
			appMode, appModeErr := p.appMode.Mode()
			if appModeErr != nil {
				return mode.Unknown, appModeErr
			}
			selectedMode = appMode
			switch appMode {
			case mode.Client:
				state = configuratorStateClient
			case mode.Server:
				state = configuratorStateServer
			default:
				return mode.Unknown, fmt.Errorf("invalid mode")
			}

		case configuratorStateClient:
			if err := p.clientConfigurator.Configure(); err != nil {
				if errors.Is(err, ErrBackToModeSelection) {
					state = configuratorStateModeSelect
					continue
				}
				return selectedMode, err
			}
			return selectedMode, nil

		case configuratorStateServer:
			if err := p.serverConfigurator.Configure(); err != nil {
				if errors.Is(err, ErrBackToModeSelection) {
					state = configuratorStateModeSelect
					continue
				}
				return selectedMode, err
			}
			return selectedMode, nil

		default:
			return mode.Unknown, fmt.Errorf("unknown configurator state: %d", state)
		}
	}
}
