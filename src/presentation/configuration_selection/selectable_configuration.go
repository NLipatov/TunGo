package configuration_selection

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"strconv"
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

	fmt.Println("Please select configuration:")
	for i := 0; i < len(configurationNames); i++ {
		if configurationNames[i] == defaultConf {
			continue
		}
		fmt.Printf("\t%s - %d\n", configurationNames[i], i)
	}
	fmt.Println("---")
	fmt.Print("Your choice: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		index, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil {
			return "", err
		}

		return configurationNames[index], nil
	}

	return "", errors.New("invalid choice")
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
