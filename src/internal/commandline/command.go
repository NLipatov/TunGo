package commandline

import (
	"errors"
	"fmt"
	"strings"
	"tungo/internal/mode"
)

type Command struct {
	Kind              CommandKind
	RuntimeMode       mode.Mode
	RequiresElevation bool
}

type CommandKind uint8

const (
	CommandUnknown CommandKind = iota
	CommandRuntime
	CommandVersion
	CommandServerConfigGenerate
)

type commandSpec struct {
	args        []string
	description string
	command     Command
}

var commands = []commandSpec{
	{
		args:        []string{"s"},
		description: "Start server runtime",
		command:     Command{Kind: CommandRuntime, RuntimeMode: mode.Server, RequiresElevation: true},
	},
	{
		args:        []string{"c"},
		description: "Start client runtime",
		command:     Command{Kind: CommandRuntime, RuntimeMode: mode.Client, RequiresElevation: true},
	},
	{
		args:        []string{"s", "gen"},
		description: "Generate server configuration",
		command:     Command{Kind: CommandServerConfigGenerate, RequiresElevation: true},
	},
	{
		args:        []string{"version"},
		description: "Show version",
		command:     Command{Kind: CommandVersion},
	},
}

// ParseCommand identifies the registered command matching the supplied arguments.
// It returns an error when the arguments do not match a supported command.
func ParseCommand(args []string) (Command, error) {
	for _, spec := range commands {
		if matches(args, spec.args) {
			return spec.command, nil
		}
	}
	return Command{}, errors.New("invalid arguments")
}

// matches reports whether two argument lists have equal lengths and corresponding values, after trimming surrounding whitespace from the actual arguments.
func matches(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			return false
		}
	}
	return true
}

// RuntimeModeArgs returns the command-line arguments for the specified runtime mode.
// It returns an error if the runtime mode is unsupported.
func RuntimeModeArgs(runtimeMode mode.Mode) ([]string, error) {
	for _, spec := range commands {
		if spec.command.Kind == CommandRuntime && spec.command.RuntimeMode == runtimeMode {
			return append([]string(nil), spec.args...), nil
		}
	}
	return nil, fmt.Errorf("unsupported runtime mode: %v", runtimeMode)
}

// Usage builds formatted usage text for the available commands.
//
// commandName identifies the executable or command name shown in the usage header.
// It returns the usage header followed by each supported command and its description.
func Usage(commandName string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Usage: %s <command>\nCommands:\n", commandName)
	for _, spec := range commands {
		_, _ = fmt.Fprintf(&b, "  %s  - %s\n", strings.Join(spec.args, " "), spec.description)
	}
	return b.String()
}
