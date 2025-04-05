package presentation

import (
	"strings"
	"tungo/domain/mode"
)

type AppMode struct {
	arguments []string
}

func NewAppMode(arguments []string) AppMode {
	return AppMode{
		arguments: arguments,
	}
}

func (a *AppMode) Mode() (mode.Mode, error) {
	if len(a.arguments) < 2 {
		return mode.Unknown, mode.NewNoModeProvided()
	}

	modeArgument := strings.TrimSpace(strings.ToLower(a.arguments[1]))
	switch modeArgument {
	case "c":
		return mode.Client, nil
	case "s":
		return mode.Server, nil
	default:
		return mode.Unknown, mode.NewInvalidModeProvided(modeArgument)
	}
}
