package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"tungo/application/confgen"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

const (
	startServerOption string = labelStartServer
	addClientOption   string = labelAddServerPeer
)

type serverConfigurator struct {
	manager         server.ConfigurationManager
	optionsSet      [2]string
	selectorFactory selector.Factory
}

type clientConfigGenerator interface {
	Generate() (*clientConfiguration.Configuration, error)
}

var (
	newServerClientConfigGenerator = func(manager server.ConfigurationManager) clientConfigGenerator {
		return confgen.NewGenerator(manager, &primitives.DefaultKeyDeriver{})
	}
	marshalServerClientConfiguration = func(v any) ([]byte, error) {
		return json.MarshalIndent(v, "", "  ")
	}
	printServerClientConfiguration = func(s string) {
		fmt.Println(s)
	}
)

type serverFlowState int

const (
	serverStateSelectOption serverFlowState = iota
	serverStateGenerateClient
)

func newServerConfigurator(manager server.ConfigurationManager, selectorFactory selector.Factory) *serverConfigurator {
	return &serverConfigurator{
		manager:         manager,
		optionsSet:      [2]string{startServerOption, addClientOption},
		selectorFactory: selectorFactory,
	}
}

func (s *serverConfigurator) Configure() error {
	return s.configureFromState(serverStateSelectOption)
}

func (s *serverConfigurator) configureFromState(state serverFlowState) error {
	for {
		switch state {
		case serverStateSelectOption:
			option, optionErr := s.selectOption()
			if optionErr != nil {
				if errors.Is(optionErr, selector.ErrNavigateBack) {
					return ErrBackToModeSelection
				}
				if errors.Is(optionErr, selector.ErrUserExit) {
					return ErrUserExit
				}
				return optionErr
			}

			switch option {
			case startServerOption:
				return nil
			case addClientOption:
				state = serverStateGenerateClient
			default:
				return fmt.Errorf("invalid option: %s", option)
			}

		case serverStateGenerateClient:
			gen := newServerClientConfigGenerator(s.manager)
			conf, err := gen.Generate()
			if err != nil {
				return err
			}
			data, err := marshalServerClientConfiguration(conf)
			if err != nil {
				return fmt.Errorf("failed to marshal client configuration: %w", err)
			}
			printServerClientConfiguration(string(data))
			state = serverStateSelectOption

		default:
			return fmt.Errorf("unknown server flow state: %d", state)
		}
	}
}

func (s *serverConfigurator) selectOption() (string, error) {
	tuiSelector, selectorErr := s.selectorFactory.NewTuiSelector(
		"Choose an option",
		s.optionsSet[:],
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if selectorErr != nil {
		return "", selectorErr
	}

	selectedOption, selectedOptionErr := tuiSelector.SelectOne()
	if selectedOptionErr != nil {
		return "", selectedOptionErr
	}
	return selectedOption, nil
}
