package command

import (
	"errors"
	"fmt"
)

var (
	ErrNoCommandProvided       = errors.New("no command provided")
	ErrInvalidExecPathProvided = errors.New("missing execution binary path as first argument")
)

type InvalidCommand string

func (i InvalidCommand) Error() string {
	if i == "" {
		return "empty string is not a valid command"
	}
	return fmt.Sprintf("%s is not a valid command", string(i))
}
