package mode_selection

import (
	"strings"
	"tungo/domain/mode"
)

type ArgsAppMode struct {
	arguments []string
}

func NewArgsAppMode(arguments []string) AppMode {
	return &ArgsAppMode{
		arguments: arguments,
	}
}

func (a *ArgsAppMode) Mode() (mode.Mode, error) {
	if len(a.arguments) == 0 {
		return mode.Unknown, mode.NewInvalidExecPathProvided()
	}

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
