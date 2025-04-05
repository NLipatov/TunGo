package mode

import "fmt"

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
	return fmt.Sprintf("%s is not a valid mode", i.mode)
}
