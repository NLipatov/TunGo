package mode_selection

import (
	"tungo/domain/mode"
	"tungo/presentation/bubble_tea"

	tea "github.com/charmbracelet/bubbletea"
)

type TeaAppMode struct {
	arguments []string
}

func NewTeaAppMode(arguments []string) AppMode {
	return &TeaAppMode{
		arguments: arguments,
	}
}

func (p *TeaAppMode) Mode() (mode.Mode, error) {
	if len(p.arguments) == 0 {
		return mode.Unknown, mode.NewInvalidExecPathProvided()
	}

	if len(p.arguments) < 2 {
		selectedMode := p.askForModeSelection()
		if selectedMode == "" {
			return mode.Unknown, mode.NewInvalidModeProvided("empty string")
		}
		p.arguments = []string{p.arguments[0], selectedMode}
	}

	appModeFromArgs := NewArgsAppMode(p.arguments)
	return appModeFromArgs.Mode()
}

func (p *TeaAppMode) askForModeSelection() string {
	selector := bubble_tea.NewSelector("Please select mode", []string{"client", "server"})
	selectorProgram, selectorProgramErr := tea.NewProgram(selector).Run()
	if selectorProgramErr != nil {
		return ""
	}

	selectorResult, ok := selectorProgram.(bubble_tea.Selector)
	if !ok {
		return ""
	}

	modeOfChoice := selectorResult.Choice()
	modeArg := string([]rune(modeOfChoice)[0])
	return modeArg
}
