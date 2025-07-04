package tui

import (
	"fmt"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/presentation/configuring/tui/components"
	"tungo/presentation/interactive_commands/handlers"
)

const (
	startServerOption string = "start server"
	addClientOption   string = "+ add a client"
)

type serverConfigurator struct {
	manager         server_configuration.ServerConfigurationManager
	optionsSet      [2]string
	selectorFactory components.SelectorFactory
}

func newServerConfigurator(manager server_configuration.ServerConfigurationManager, selectorFactory components.SelectorFactory) *serverConfigurator {
	return &serverConfigurator{
		manager:         manager,
		optionsSet:      [2]string{startServerOption, addClientOption},
		selectorFactory: selectorFactory,
	}
}

func (s *serverConfigurator) Configure() error {
	option, optionErr := s.selectOption()
	if optionErr != nil {
		return optionErr
	}

	switch option {
	case startServerOption:
		return nil
	case addClientOption:
		handler := handlers.NewConfgenHandler(s.manager)
		generateNewClientConfErr := handler.GenerateNewClientConf()
		if generateNewClientConfErr != nil {
			return generateNewClientConfErr
		}
		return s.Configure()
	default:
		return fmt.Errorf("invalid option: %s", option)
	}
}

func (s *serverConfigurator) selectOption() (string, error) {
	selector, selectorErr := s.selectorFactory.NewTuiSelector("Choose an option", s.optionsSet[:])
	if selectorErr != nil {
		return "", selectorErr
	}

	selectedOption, selectedOptionErr := selector.SelectOne()
	if selectedOptionErr != nil {
		return "", selectedOptionErr
	}
	return selectedOption, nil
}
