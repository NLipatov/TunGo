package configuration_selection

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"tungo/settings/client_configuration"
)

type SelectableConfiguration struct {
	resolver net.Resolver
	observer client_configuration.Observer
	selector client_configuration.Selector
}

func NewSelectableConfiguration(observer client_configuration.Observer, selector client_configuration.Selector) *SelectableConfiguration {
	return &SelectableConfiguration{
		observer: observer,
		selector: selector,
	}
}

func (p *SelectableConfiguration) SelectConfiguration() error {
	configurationOptions, configurationOptionsErr := p.observer.Observe()
	if configurationOptionsErr != nil {
		return configurationOptionsErr
	}

	// if there's only one option to choose from - use it
	if len(configurationOptions) == 1 {
		selectErr := p.selector.Select(configurationOptions[0])
		return selectErr
	}

	// if there's more than one option - prompt user to choose configuration
	if len(configurationOptions) > 1 {
		confName, confNameErr := p.promptForConfigurations(configurationOptions)
		if confNameErr != nil {
			return confNameErr
		}

		selectErr := p.selector.Select(confName)
		return selectErr
	}

	// use default configuration
	return nil
}

func (p *SelectableConfiguration) promptForConfigurations(configurationNames []string) (string, error) {
	fmt.Println("Please select configuration:")
	for i := 0; i < len(configurationNames); i++ {
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
