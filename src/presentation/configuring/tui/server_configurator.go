package tui

import (
	"fmt"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
	"tungo/presentation/interactive_commands/handlers"
)

const (
	startServerOption string = "start server"
	addClientOption   string = "+ add a client"
)

type serverConfigurator struct {
	manager         server.ConfigurationManager
	optionsSet      [2]string
	selectorFactory selector.Factory
}

func newServerConfigurator(manager server.ConfigurationManager, selectorFactory selector.Factory) *serverConfigurator {
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
		handler := handlers.NewConfgenHandler(s.manager, handlers.NewJsonMarshaller())
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
