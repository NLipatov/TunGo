package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/presentation/configuring/tui/components"
)

const (
	addOption    string = "+ add configuration"
	removeOption string = "- remove configuration"
)

type clientConfigurator struct {
	observer client_configuration.Observer
	selector client_configuration.Selector
	deleter  client_configuration.Deleter
	creator  client_configuration.Creator
}

func newClientConfigurator(observer client_configuration.Observer,
	selector client_configuration.Selector,
	deleter client_configuration.Deleter,
	creator client_configuration.Creator) *clientConfigurator {
	return &clientConfigurator{
		observer: observer,
		selector: selector,
		deleter:  deleter,
		creator:  creator,
	}
}

func (c *clientConfigurator) Configure() error {
	options, optionsErr := c.observer.Observe()
	if optionsErr != nil {
		return optionsErr
	}

	// deletion option is only shown if there's anything to delete
	if len(options) > 0 {
		options = append(options, removeOption)
	}
	//add option is always shown
	options = append(options, addOption)

	selectedOption, selectedOptionErr := c.selectConf(options, "Select configuration – or add/remove one:")
	if selectedOptionErr != nil {
		return selectedOptionErr
	}

	if selectedOption == removeOption {
		optionsWithoutAddAndRemove := options[:len(options)-2]
		confToDelete, confToDeleteErr := c.selectConf(optionsWithoutAddAndRemove, "Choose a configuration to remove:")
		if confToDeleteErr != nil {
			return confToDeleteErr
		}

		deleteErr := c.deleter.Delete(confToDelete)
		if deleteErr != nil {
			return deleteErr
		}

		if len(options) == 1 {
			createErr := c.createConf()
			if createErr != nil {
				return createErr
			}

			return c.Configure()
		}

		return c.Configure()
	} else if selectedOption == addOption {
		createErr := c.createConf()
		if createErr != nil {
			return createErr
		}

		return c.Configure()
	}

	selectErr := c.selector.Select(selectedOption)
	return selectErr
}

func (c *clientConfigurator) selectConf(configurationNames []string, placeholder string) (string, error) {
	options := make([]string, len(configurationNames))
	optionsIndex := 0
	for _, confName := range configurationNames {
		options[optionsIndex] = confName
		optionsIndex++
	}
	options = options[:optionsIndex]

	selector := components.NewSelector(placeholder, options)
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

func (c *clientConfigurator) createConf() error {
	textInput := components.NewTextInput("Give it a name")
	textInputProgram, textInputProgramErr := tea.NewProgram(textInput).Run()
	if textInputProgramErr != nil {
		return textInputProgramErr
	}

	textInputResult, textInputResulOk := textInputProgram.(*components.TextInput)
	if !textInputResulOk {
		return errors.New("invalid textInput format")
	}

	textArea := components.NewTextArea("Paste it here")
	textAreaProgram, textAreaProgramErr := tea.
		NewProgram(textArea, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout)).Run()
	if textAreaProgramErr != nil {
		return textAreaProgramErr
	}

	textAreaResult, ok := textAreaProgram.(*components.TextArea)
	if !ok {
		return errors.New("unexpected textArea type")
	}

	configurationParser := NewConfigurationParser()
	configuration, configurationErr := configurationParser.FromJson(textAreaResult.Value())
	if configurationErr != nil {
		return configurationErr
	}

	return c.creator.Create(configuration, textInputResult.Value())
}
