package cli

import (
	"fmt"
	"os"
	"strings"
	"tungo/domain/app"
	"tungo/domain/mode"
)

const (
	ServerMode        = "s"
	ServerConfGenMode = "s gen"
	ClientMode        = "c"
)

type Configurator struct {
}

func NewConfigurator() *Configurator {
	return &Configurator{}
}

func (c *Configurator) Configure() (mode.Mode, error) {
	if len(os.Args) < 2 {
		c.printUsage()
		return mode.Unknown, fmt.Errorf("invalid arguments")
	}

	switch strings.Join(c.trimArgs(os.Args[1:]), " ") {
	case ClientMode:
		return mode.Client, nil
	case ServerMode:
		return mode.Server, nil
	case ServerConfGenMode:
		return mode.ServerConfGen, nil
	default:
		c.printUsage()
		return mode.Unknown, fmt.Errorf("invalid arguments")
	}
}

func (c *Configurator) trimArgs(args []string) []string {
	for i, v := range args {
		args[i] = strings.TrimSpace(v)
	}

	return args
}

func (c *Configurator) printUsage() {
	fmt.Printf(`Usage: %s <mode>
Modes:
  %s  - Server
  %s  - Client
`, app.Name, ServerMode, ClientMode)
}
