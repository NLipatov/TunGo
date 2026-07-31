package tui

import (
	"context"
	"errors"
	"fmt"

	"tungo/application/runtime"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

	tea "charm.land/bubbletea/v2"
)

func (t *TUI) configure(ctx context.Context, logFeed bubbleTea.RuntimeLogFeed) (runtime.Mode, error) {
	options := t.configuratorOptions
	options.LogFeed = logFeed
	model, err := bubbleTea.NewConfigurator(options, t.preferences)
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
