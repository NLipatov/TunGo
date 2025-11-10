package tui

import (
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/presentation/configuring/tui/components"
)

const (
	addOption    string = "+ add configuration"
	removeOption string = "- remove configuration"
)

type clientConfigurator struct {
	observer         clientConfiguration.Observer
	selector         clientConfiguration.Selector
	deleter          clientConfiguration.Deleter
	creator          clientConfiguration.Creator
	selectorFactory  components.SelectorFactory
	textInputFactory components.TextInputFactory
	textAreaFactory  components.TextAreaFactory
}

func newClientConfigurator(observer clientConfiguration.Observer,
	selector clientConfiguration.Selector,
	deleter clientConfiguration.Deleter,
	creator clientConfiguration.Creator,
	selectorFactory components.SelectorFactory,
	textInputFactory components.TextInputFactory,
	textAreaFactory components.TextAreaFactory) *clientConfigurator {
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

	selectedOption, selectedOptionErr := c.selectConf(
		options,
		"Select configuration – or add/remove one:",
		components.NewDefaultColor(), components.NewTransparentColor(),
	)
	if selectedOptionErr != nil {
		return selectedOptionErr
	}

	if selectedOption == removeOption {
		optionsWithoutAddAndRemove := options[:len(options)-2]
		confToDelete, confToDeleteErr := c.selectConf(
			optionsWithoutAddAndRemove,
			"Choose a configuration to remove:",
			components.NewColor(255, 0, 0, true), components.NewTransparentColor(),
		)
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

func (c *clientConfigurator) selectConf(
	configurationNames []string,
	placeholder string,
	foreground, background components.Color,
) (string, error) {
	options := make([]string, len(configurationNames))
	optionsIndex := 0
	for _, confName := range configurationNames {
		options[optionsIndex] = confName
		optionsIndex++
	}
	options = options[:optionsIndex]

	selector, selectorErr := c.selectorFactory.NewTuiSelector(
		placeholder,
		options,
		foreground, background,
	)
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
