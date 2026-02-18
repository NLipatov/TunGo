package tui

import (
	"errors"
	"fmt"
	"strings"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

const (
	addOption                 string = labelAddConfig
	removeOption              string = labelRemoveConfig
	removeItemPrefix          string = ""
	invalidConfigDeleteOption string = labelDeleteInvalid
	invalidConfigOkOption     string = "OK"
)

type clientConfigurator struct {
	observer             clientConfiguration.Observer
	selector             clientConfiguration.Selector
	deleter              clientConfiguration.Deleter
	creator              clientConfiguration.Creator
	selectorFactory      selector.Factory
	textInputFactory     text_input.TextInputFactory
	textAreaFactory      text_area.TextAreaFactory
	configurationManager clientConfiguration.ConfigurationManager
}

type clientFlowState int

const (
	clientStateSelectConfiguration clientFlowState = iota
	clientStateSelectForRemoval
	clientStateAddName
	clientStateAddJSON
	clientStateValidateSelection
	clientStateInvalidConfigWarning
)

type clientFlowContext struct {
	state                 clientFlowState
	configurationOptions  []string
	selectedConfiguration string
	newConfigurationName  string
	invalidConfiguration  string
	invalidErr            error
	allowInvalidDelete    bool
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
		observer:             observer,
		selector:             selector,
		deleter:              deleter,
		creator:              creator,
		selectorFactory:      selectorFactory,
		textInputFactory:     textInputFactory,
		textAreaFactory:      textAreaFactory,
		configurationManager: configurationManager,
	}
}

func (c *clientConfigurator) Configure() error {
	return c.configureFromState(clientStateSelectConfiguration)
}

func (c *clientConfigurator) configureFromState(state clientFlowState) error {
	flow := clientFlowContext{state: state}
	for {
		switch flow.state {
		case clientStateSelectConfiguration:
			configurationOptions, optionsErr := c.observer.Observe()
			if optionsErr != nil {
				return optionsErr
			}
			flow.configurationOptions = configurationOptions

			options := make([]string, 0, len(configurationOptions)+2)
			options = append(options, configurationOptions...)
			if len(configurationOptions) > 0 {
				options = append(options, removeOption)
			}
			options = append(options, addOption)

			selectedOption, selectedOptionErr := c.selectConf(
				options,
				"Select configuration - or add/remove one:",
				value_objects.NewDefaultColor(), value_objects.NewTransparentColor(),
			)
			if selectedOptionErr != nil {
				switch {
				case errors.Is(selectedOptionErr, selector.ErrNavigateBack):
					return ErrBackToModeSelection
				case errors.Is(selectedOptionErr, selector.ErrUserExit):
					return ErrUserExit
				default:
					return selectedOptionErr
				}
			}

			switch selectedOption {
			case removeOption:
				flow.state = clientStateSelectForRemoval
			case addOption:
				flow.state = clientStateAddName
			default:
				flow.selectedConfiguration = selectedOption
				flow.state = clientStateValidateSelection
			}

		case clientStateSelectForRemoval:
			removeOptions, removeOptionToConfig := buildRemoveOptions(flow.configurationOptions)
			confToDelete, confToDeleteErr := c.selectConf(
				removeOptions,
				"Choose a configuration to remove:",
				value_objects.NewColor(value_objects.ColorRed, true), value_objects.NewTransparentColor(),
			)
			if confToDeleteErr != nil {
				switch {
				case errors.Is(confToDeleteErr, selector.ErrNavigateBack):
					flow.state = clientStateSelectConfiguration
					continue
				case errors.Is(confToDeleteErr, selector.ErrUserExit):
					return ErrUserExit
				default:
					return confToDeleteErr
				}
			}

			resolvedConfToDelete, ok := removeOptionToConfig[confToDelete]
			if !ok {
				return fmt.Errorf("configuration selection aborted")
			}
			if deleteErr := c.deleter.Delete(resolvedConfToDelete); deleteErr != nil {
				return deleteErr
			}

			if len(flow.configurationOptions) <= 1 {
				flow.state = clientStateAddName
			} else {
				flow.state = clientStateSelectConfiguration
			}

		case clientStateAddName:
			textInput, valueErr := c.textInputFactory.NewTextInput("Give it a name")
			if valueErr != nil {
				return valueErr
			}
			textInputValue, textInputValueErr := textInput.Value()
			if textInputValueErr != nil {
				if errors.Is(textInputValueErr, text_input.ErrCancelled) {
					flow.state = clientStateSelectConfiguration
					continue
				}
				return textInputValueErr
			}
			flow.newConfigurationName = textInputValue
			flow.state = clientStateAddJSON

		case clientStateAddJSON:
			textArea, textAreaErr := c.textAreaFactory.NewTextArea("Paste it here")
			if textAreaErr != nil {
				return textAreaErr
			}
			textAreaValue, textAreaValueErr := textArea.Value()
			if textAreaValueErr != nil {
				if errors.Is(textAreaValueErr, text_area.ErrCancelled) {
					flow.state = clientStateAddName
					continue
				}
				return textAreaValueErr
			}

			configurationParser := NewConfigurationParser()
			configuration, configurationErr := configurationParser.FromJson(textAreaValue)
			if configurationErr != nil {
				flow.invalidErr = configurationErr
				flow.invalidConfiguration = ""
				flow.allowInvalidDelete = false
				flow.state = clientStateInvalidConfigWarning
				continue
			}

			if createErr := c.creator.Create(configuration, flow.newConfigurationName); createErr != nil {
				return createErr
			}
			flow.state = clientStateSelectConfiguration

		case clientStateValidateSelection:
			if selectErr := c.selector.Select(flow.selectedConfiguration); selectErr != nil {
				return selectErr
			}
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

			flow.invalidErr = configurationErr
			flow.invalidConfiguration = flow.selectedConfiguration
			flow.allowInvalidDelete = true
			flow.state = clientStateInvalidConfigWarning

		case clientStateInvalidConfigWarning:
			if warningErr := c.showInvalidConfigurationWarning(flow.invalidConfiguration, flow.invalidErr, flow.allowInvalidDelete); warningErr != nil {
				return warningErr
			}
			flow.state = clientStateSelectConfiguration

		default:
			return fmt.Errorf("unknown client flow state: %d", flow.state)
		}
	}
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

func buildRemoveOptions(configurationNames []string) ([]string, map[string]string) {
	options := make([]string, 0, len(configurationNames))
	optionToConfig := make(map[string]string, len(configurationNames))
	for _, name := range configurationNames {
		display := removeItemPrefix + name
		options = append(options, display)
		optionToConfig[display] = name
	}
	return options, optionToConfig
}

func (c *clientConfigurator) showInvalidConfigurationWarning(selectedConfiguration string, configurationErr error, allowDelete bool) error {
	reason := summarizeInvalidConfigurationError(configurationErr)
	placeholder := fmt.Sprintf("Configuration is invalid: %s", reason)
	options := []string{invalidConfigOkOption}
	if allowDelete {
		options = []string{invalidConfigDeleteOption, invalidConfigOkOption}
	}
	selectedOption, selectErr := c.selectConf(
		options,
		placeholder,
		value_objects.NewColor(value_objects.ColorRed, true),
		value_objects.NewTransparentColor(),
	)
	if selectErr != nil {
		if errors.Is(selectErr, selector.ErrNavigateBack) {
			return nil
		}
		if errors.Is(selectErr, selector.ErrUserExit) {
			return ErrUserExit
		}
		return selectErr
	}

	switch selectedOption {
	case invalidConfigDeleteOption:
		if !allowDelete {
			return fmt.Errorf("configuration selection aborted")
		}
		if c.deleter == nil {
			return fmt.Errorf("invalid configuration cannot be deleted")
		}
		if selectedConfiguration == "" {
			return fmt.Errorf("configuration selection aborted")
		}
		if deleteErr := c.deleter.Delete(selectedConfiguration); deleteErr != nil {
			return deleteErr
		}
		return nil
	case invalidConfigOkOption:
		return nil
	default:
		return fmt.Errorf("configuration selection aborted")
	}
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
