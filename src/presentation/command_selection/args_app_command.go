package command_selection

import (
	"strings"
	"tungo/domain/command"
)

type ArgsAppCommand struct {
	arguments []string
}

func NewArgsAppCommand(arguments []string) AppCommand {
	return &ArgsAppCommand{
		arguments: arguments,
	}
}

func (a *ArgsAppCommand) Command() (command.Command, error) {
	if len(a.arguments) == 0 {
		return command.Unknown, command.ErrInvalidExecPathProvided
	}

	if len(a.arguments) < 2 {
		return command.Unknown, command.ErrNoCommandProvided
	}

	commandArgument := strings.TrimSpace(strings.ToLower(a.arguments[1]))
	switch commandArgument {
	case "c":
		return command.StartClient, nil
	case "s":
		return command.StartServer, nil
	default:
		return command.Unknown, command.InvalidCommand(commandArgument)
	}
}
