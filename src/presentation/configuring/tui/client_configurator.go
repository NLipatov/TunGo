package tui

import (
	"fmt"
	"strings"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

const (
	addOption                 string = "+ add configuration"
	removeOption              string = "- remove configuration"
	invalidConfigRetryOption  string = "Choose another configuration"
	invalidConfigDetailOption string = "Show details"
	invalidConfigBackOption   string = "Back"
)

type clientConfigurator struct {
	observer              clientConfiguration.Observer
	selector              clientConfiguration.Selector
	deleter               clientConfiguration.Deleter
	creator               clientConfiguration.Creator
	selectorFactory       selector.Factory
	textInputFactory      text_input.TextInputFactory
	textAreaFactory       text_area.TextAreaFactory
	configurationManager  clientConfiguration.ConfigurationManager
	invalidConfigHeadline string
}

func newClientConfigurator(observer clientConfiguration.Observer,
	selector clientConfiguration.Selector,
	deleter clientConfiguration.Deleter,
	creator clientConfiguration.Creator,
	selectorFactory selector.Factory,
	textInputFactory text_input.TextInputFactory,
	textAreaFactory text_area.TextAreaFactory,
	configurationManager clientConfiguration.ConfigurationManager) *clientConfigurator {
	return &clientConfigurator{
		observer:              observer,
		selector:              selector,
		deleter:               deleter,
		creator:               creator,
		selectorFactory:       selectorFactory,
		textInputFactory:      textInputFactory,
		textAreaFactory:       textAreaFactory,
		configurationManager:  configurationManager,
		invalidConfigHeadline: "Configuration error",
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
		value_objects.NewDefaultColor(), value_objects.NewTransparentColor(),
	)
	if selectedOptionErr != nil {
		return selectedOptionErr
	}

	if selectedOption == removeOption {
		optionsWithoutAddAndRemove := options[:len(options)-2]
		confToDelete, confToDeleteErr := c.selectConf(
			optionsWithoutAddAndRemove,
			"Choose a configuration to remove:",
			value_objects.NewColor(value_objects.ColorGreen, true), value_objects.NewTransparentColor(),
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
	if selectErr != nil {
		return selectErr
	}

	return c.ensureSelectedConfigurationIsValid()
}

func (c *clientConfigurator) selectConf(
	configurationNames []string,
	placeholder string,
	foreground, background value_objects.Color,
) (string, error) {
	options := make([]string, len(configurationNames))
	optionsIndex := 0
	for _, confName := range configurationNames {
		options[optionsIndex] = confName
		optionsIndex++
	}
	options = options[:optionsIndex]

	tuiSelector, selectorErr := c.selectorFactory.NewTuiSelector(
		placeholder,
		options,
		foreground, background,
	)
	if selectorErr != nil {
		return "", selectorErr
	}

	selectedOption, selectOneErr := tuiSelector.SelectOne()
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

func (c *clientConfigurator) ensureSelectedConfigurationIsValid() error {
	if c.configurationManager == nil {
		return nil
	}

	_, configurationErr := c.configurationManager.Configuration()
	if configurationErr == nil {
		return nil
	}
	if !isInvalidClientConfigurationError(configurationErr) {
		return configurationErr
	}

	return c.showInvalidConfigurationWarning(configurationErr)
}

func (c *clientConfigurator) showInvalidConfigurationWarning(configurationErr error) error {
	reason := summarizeInvalidConfigurationError(configurationErr)
	for {
		placeholder := fmt.Sprintf(
			"%s\nSelected configuration is invalid. Choose another configuration to continue.\nReason: %s",
			c.invalidConfigHeadline,
			reason,
		)
		selectedOption, selectErr := c.selectConf(
			[]string{invalidConfigRetryOption, invalidConfigDetailOption},
			placeholder,
			value_objects.NewColor(value_objects.ColorRed, true),
			value_objects.NewTransparentColor(),
		)
		if selectErr != nil {
			return selectErr
		}

		switch selectedOption {
		case invalidConfigRetryOption:
			return c.Configure()
		case invalidConfigDetailOption:
			if detailErr := c.showInvalidConfigurationDetails(configurationErr); detailErr != nil {
				return detailErr
			}
		default:
			return fmt.Errorf("configuration selection aborted")
		}
	}
}

func (c *clientConfigurator) showInvalidConfigurationDetails(configurationErr error) error {
	selectedOption, selectErr := c.selectConf(
		[]string{invalidConfigBackOption},
		fmt.Sprintf("%s details\n%s", c.invalidConfigHeadline, strings.TrimSpace(configurationErr.Error())),
		value_objects.NewColor(value_objects.ColorRed, true),
		value_objects.NewTransparentColor(),
	)
	if selectErr != nil {
		return selectErr
	}
	if selectedOption != invalidConfigBackOption {
		return fmt.Errorf("configuration selection aborted")
	}
	return nil
}

func summarizeInvalidConfigurationError(err error) string {
	if err == nil {
		return ""
	}

	message := strings.TrimSpace(err.Error())
	normalized := strings.ToLower(message)
	if strings.Contains(normalized, "invalid client configuration (") {
		if separatorIdx := strings.Index(message, "): "); separatorIdx >= 0 && separatorIdx+3 <= len(message) {
			message = message[separatorIdx+3:]
		}
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 120 {
		return message[:117] + "..."
	}
	return message
}

func isInvalidClientConfigurationError(err error) bool {
	if err == nil {
		return false
	}

	normalized := strings.ToLower(err.Error())
	invalidMessages := []string{
		"invalid client configuration",
		"invalid character",
		"cannot unmarshal",
		"unexpected eof",
	}
	for _, message := range invalidMessages {
		if strings.Contains(normalized, message) {
			return true
		}
	}

	return false
}
