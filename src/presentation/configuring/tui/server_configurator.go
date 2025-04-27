package tui

import (
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"tungo/presentation/configuring/tui/components"
	"tungo/presentation/interactive_commands/handlers"
	"tungo/settings/server_configuration"
)

const (
	startServerOption string = "start server"
	addClientOption   string = "+ add a client"
)

type serverConfigurator struct {
	manager    server_configuration.ServerConfigurationManager
	optionsSet [2]string
}

func newServerConfigurator(manager server_configuration.ServerConfigurationManager) *serverConfigurator {
	return &serverConfigurator{
		manager:    manager,
		optionsSet: [2]string{startServerOption, addClientOption},
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
		generateNewClientConfErr := handlers.GenerateNewClientConf()
		if generateNewClientConfErr != nil {
			return generateNewClientConfErr
		}
		return s.Configure()
	default:
		return fmt.Errorf("invalid option: %s", option)
	}
}

func (s *serverConfigurator) selectOption() (string, error) {
	selector := components.NewSelector("Choose an option", s.optionsSet[:])
	selectorProgram, selectorProgramErr := tea.NewProgram(selector).Run()
	if selectorProgramErr != nil {
		return "", selectorProgramErr
	}

	selectorResult, ok := selectorProgram.(components.Selector)
	if !ok {
		return "", errors.New("invalid selector format")
	}

	return selectorResult.Choice(), nil
}
