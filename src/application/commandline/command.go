package commandline

import (
	"errors"
	"fmt"
	"strings"
	"tungo/runtime"
)

type CommandKind uint8

const (
	CommandUnknown CommandKind = iota
	CommandRuntime
	CommandVersion
	CommandServerConfigGenerate
)

type Command struct {
	Kind              CommandKind
	RuntimeMode       runtime.Mode
	RequiresElevation bool
}

type commandSpec struct {
	args        []string
	description string
	command     Command
}

var commands = []commandSpec{
	{
		args:        []string{"s"},
		description: "Start server runtime",
		command:     Command{Kind: CommandRuntime, RuntimeMode: runtime.ModeServer, RequiresElevation: true},
	},
	{
		args:        []string{"c"},
		description: "Start client runtime",
		command:     Command{Kind: CommandRuntime, RuntimeMode: runtime.ModeClient, RequiresElevation: true},
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

func ParseCommand(args []string) (Command, error) {
	for _, spec := range commands {
		if matches(args, spec.args) {
			return spec.command, nil
		}
	}
	return Command{}, errors.New("invalid arguments")
}

func RuntimeModeArgs(mode runtime.Mode) ([]string, error) {
	for _, spec := range commands {
		if spec.command.Kind == CommandRuntime && spec.command.RuntimeMode == mode {
			return append([]string(nil), spec.args...), nil
		}
	}
	return nil, fmt.Errorf("unsupported runtime mode: %v", mode)
}

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

func CommandUsage(commandName string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Usage: %s <command>\nCommands:\n", commandName)
	for _, spec := range commands {
		_, _ = fmt.Fprintf(&b, "  %s  - %s\n", strings.Join(spec.args, " "), spec.description)
	}
	return b.String()
}
