package cli

import (
	"fmt"
	"os"
	"tungo/domain/app"
	"tungo/domain/mode"
)

const (
	ServerMode = "s"
	ClientMode = "c"
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

	switch os.Args[1] {
	case "c":
		return mode.Client, nil
	case "s":
		return mode.Server, nil
	default:
		c.printUsage()
		return mode.Unknown, fmt.Errorf("invalid arguments")
	}
}

func (c *Configurator) printUsage() {
	fmt.Printf(`Usage: %s <mode>
Modes:
  %s  - Server
  %s  - Client
`, app.Name, ServerMode, ClientMode)
}
