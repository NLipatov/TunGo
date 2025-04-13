package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"tungo/domain/mode"
	"tungo/presentation/configuring/tui/components"
)

type AppMode struct {
}

func NewAppMode() AppMode {
	return AppMode{}
}

func (p *AppMode) Mode() (mode.Mode, error) {
	clientMode := "client"
	serverMode := "server"
	selector := components.NewSelector("Mode selection:", []string{clientMode, serverMode})
	selectorProgram, selectorProgramErr := tea.NewProgram(selector).Run()
	if selectorProgramErr != nil {
		return mode.Unknown, selectorProgramErr
	}

	selectorResult, ok := selectorProgram.(components.Selector)
	if !ok {
		return mode.Unknown, fmt.Errorf("could not cast selector")
	}

	appMode := selectorResult.Choice()
	switch appMode {
	case clientMode:
		return mode.Client, nil
	case serverMode:
		return mode.Server, nil
	default:
		return mode.Unknown, fmt.Errorf("invalid mode argument")
	}
}
