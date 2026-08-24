package tui

import (
	"context"
	"errors"
	"fmt"

	"tungo/internal/mode"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"

	tea "charm.land/bubbletea/v2"
)

func (t *TUI) configure(ctx context.Context, logFeed bubbleTea.RuntimeLogFeed) (mode.Mode, error) {
	var serverConfigurations bubbleTea.ServerConfigurations
	if t.serverFile != nil {
		serverConfigurations = t.serverFile
	}
	model, err := bubbleTea.NewConfigurator(bubbleTea.ConfiguratorOptions{
		ClientConfigurations: t.clientConfigurations,
		ServerConfigurations: serverConfigurations,
		Daemon:               t.daemonControl,
		LogFeed:              logFeed,
	}, t.preferences)
	if err != nil {
		return 0, err
	}

	program := tea.NewProgram(model)
	stopQuit := context.AfterFunc(ctx, program.Quit)
	finalModel, err := program.Run()
	stopQuit()
	if err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		return 0, ErrUserExit
	}

	configurator, ok := finalModel.(bubbleTea.Configurator)
	if !ok {
		return 0, fmt.Errorf("unexpected configurator model: %T", finalModel)
	}
	mode, err := configurator.Result()
	if errors.Is(err, bubbleTea.ErrConfiguratorUserExit) {
		return 0, ErrUserExit
	}
	return mode, err
}
