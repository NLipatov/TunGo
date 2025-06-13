package tui

import (
	"tungo/infrastructure/PAL/client_configuration"
)

const (
	addOption    string = "+ add configuration"
	removeOption string = "- remove configuration"
)

type clientConfigurator struct {
	observer         client_configuration.Observer
	selector         client_configuration.Selector
	deleter          client_configuration.Deleter
	creator          client_configuration.Creator
	selectorFactory  SelectorFactory
	textInputFactory TextInputFactory
	textAreaFactory  TextAreaFactory
}

func newClientConfigurator(observer client_configuration.Observer,
	selector client_configuration.Selector,
	deleter client_configuration.Deleter,
	creator client_configuration.Creator,
	selectorFactory SelectorFactory,
	textInputFactory TextInputFactory,
	textAreaFactory TextAreaFactory) *clientConfigurator {
	return &clientConfigurator{
		observer:         observer,
		selector:         selector,
		deleter:          deleter,
		creator:          creator,
		selectorFactory:  selectorFactory,
		textInputFactory: textInputFactory,
		textAreaFactory:  textAreaFactory,
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

	selector, selectorErr := c.selectorFactory.NewTuiSelector(placeholder, options)
	if selectorErr != nil {
		return "", selectorErr
	}

	selectedOption, selectOneErr := selector.SelectOne()
	if selectOneErr != nil {
		return "", selectOneErr
	}

	return selectedOption, nil
}

func (c *clientConfigurator) createConf() error {
	textInput, valueErr := c.textInputFactory.NewTextInput("Give it a name")
	if valueErr != nil {
		return valueErr
	}

	textInputValue, textInputValueErr := textInput.Value()
	if textInputValueErr != nil {
		return textInputValueErr
	}

	textArea, textAreaErr := c.textAreaFactory.NewTextArea("Paste it here")
	if textAreaErr != nil {
		return textAreaErr
	}

	textAreaValue, textAreaValueErr := textArea.Value()
	if textAreaValueErr != nil {
		return textAreaValueErr
	}

	configurationParser := NewConfigurationParser()
	configuration, configurationErr := configurationParser.FromJson(textAreaValue)
	if configurationErr != nil {
		return configurationErr
	}

	return c.creator.Create(configuration, textInputValue)
}
