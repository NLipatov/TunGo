package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"tungo/domain/app"
	"tungo/domain/command"
)

const (
	ServerMode        = "s"
	ServerConfGenMode = "s gen"
	ClientMode        = "c"
	Version           = "version"
	Help              = "help"
	HelpShort         = "-h"
	HelpLong          = "--help"
)

type Configurator struct {
}

func NewConfigurator() *Configurator {
	return &Configurator{}
}

func (c *Configurator) Configure(_ context.Context) (command.Command, error) {
	if app.CurrentUIMode() == app.TUI {
		c.printHelp()
		return command.Unknown, fmt.Errorf("invalid arguments")
	}

	args := c.trimArgs(os.Args[1:])
	switch strings.Join(args, " ") {
	case ClientMode:
		return command.StartClient, nil
	case ServerMode:
		return command.StartServer, nil
	case ServerConfGenMode:
		return command.GenerateClientConfig, nil
	case Version:
		return command.ShowVersion, nil
	default:
		c.printHelp()
		return command.Unknown, fmt.Errorf("invalid arguments")
	}
}

func (c *Configurator) trimArgs(args []string) []string {
	for i, v := range args {
		args[i] = strings.TrimSpace(v)
	}

	return args
}

func IsHelpRequest(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch strings.TrimSpace(args[0]) {
	case Help, HelpShort, HelpLong:
		return true
	default:
		return false
	}
}

func HelpText() string {
	return fmt.Sprintf(`Usage:
  %s <command>

Commands:
  %-7s  Start a server
  %-7s  Start a client
  %-7s  Generate client configuration
  %-7s  Show version
  %-7s  Show help

Options:
  -h, --help  Show help
`, app.Name, ServerMode, ClientMode, ServerConfGenMode, Version, Help)
}

func PrintHelp() {
	fmt.Print(HelpText())
}

func (c *Configurator) printHelp() {
	PrintHelp()
}
