package configuration_selection

import (
	"encoding/json"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"strings"
	"tungo/presentation/bubble_tea"
	"tungo/settings/client_configuration"
)

type SelectableConfiguration struct {
	resolver client_configuration.Resolver
	observer client_configuration.Observer
	selector client_configuration.Selector
	creator  client_configuration.Creator
}

func NewSelectableConfiguration(
	observer client_configuration.Observer,
	selector client_configuration.Selector,
	creator client_configuration.Creator,
	resolver client_configuration.Resolver,
) *SelectableConfiguration {
	return &SelectableConfiguration{
		observer: observer,
		selector: selector,
		creator:  creator,
		resolver: resolver,
	}
}

func (p *SelectableConfiguration) SelectConfiguration() error {
	options, optionsErr := p.observer.Observe()
	if optionsErr != nil {
		return optionsErr
	}

	// if there's only one option to choose from - use it
	if len(options) == 1 {
		selectErr := p.selector.Select(options[0])
		return selectErr
	}

	// if there's more than one option - prompt user to choose configuration
	if len(options) > 1 {
		confName, confNameErr := p.selectConf(options)
		if confNameErr != nil {
			return confNameErr
		}

		selectErr := p.selector.Select(confName)
		return selectErr
	}

	if len(options) == 0 {
		return p.createConf()
	}

	// use default configuration
	return nil
}

func (p *SelectableConfiguration) selectConf(configurationNames []string) (string, error) {
	defaultConf, defaultConfErr := p.resolver.Resolve()
	if defaultConfErr != nil {
		return "", defaultConfErr
	}

	options := make([]string, len(configurationNames))
	optionsIndex := 0
	for _, confName := range configurationNames {
		if confName == defaultConf {
			continue
		}

		options[optionsIndex] = confName
		optionsIndex++
	}
	options = options[:optionsIndex]

	selector := bubble_tea.NewSelector("Please select configuration to use", options)
	selectorProgram, selectorProgramErr := tea.NewProgram(selector).Run()
	if selectorProgramErr != nil {
		return "", selectorProgramErr
	}

	selectorResult, ok := selectorProgram.(bubble_tea.Selector)
	if !ok {
		return "", errors.New("invalid selector format")
	}

	return selectorResult.Choice(), nil
}

func (p *SelectableConfiguration) createConf() error {
	textArea := bubble_tea.NewTextArea("Please paste your configuration:")
	textAreaProgram, textAreaProgramErr := tea.
		NewProgram(textArea, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout)).Run()
	if textAreaProgramErr != nil {
		return textAreaProgramErr
	}

	textAreaResult, ok := textAreaProgram.(*bubble_tea.TextArea)
	if !ok {
		return errors.New("unexpected textArea type")
	}

	jsonText := textAreaResult.Value()
	var configuration client_configuration.Configuration
	if configurationErr := json.
		Unmarshal([]byte(strings.TrimSpace(jsonText)), &configuration); configurationErr != nil {
		return configurationErr
	}

	return p.creator.Create(configuration)
}
