package mode

import (
	"fmt"
)

// NoModeProvided is returned when no Mode was provided when running an app
type NoModeProvided struct {
}

func NewNoModeProvided() NoModeProvided {
	return NoModeProvided{}
}

func (n NoModeProvided) Error() string {
	return "no mode provided"
}

type InvalidModeProvided struct {
	mode string
}

func NewInvalidModeProvided(mode string) InvalidModeProvided {
	return InvalidModeProvided{
		mode: mode,
	}
}

func (i InvalidModeProvided) Error() string {
	if i.mode == "" {
		return "empty string is not a valid mode"
	}

	return fmt.Sprintf("%s is not a valid mode", i.mode)
}

type InvalidExecPathProvided struct {
}

func NewInvalidExecPathProvided() InvalidExecPathProvided {
	return InvalidExecPathProvided{}
}

func (i InvalidExecPathProvided) Error() string {
	return "missing execution binary path as first argument"
}
