package tui

import (
	"context"
	"errors"
	"fmt"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_area"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_input"
	uifactory "tungo/presentation/ui/tui/internal/ui/factory"
)

// unifiedSessionHandle is the subset of *bubbleTea.UnifiedSession used by the configurator
// and runtime backend. Extracted as an interface for testability.
type unifiedSessionHandle interface {
	WaitForMode() (mode.Mode, error)
	ActivateRuntime(ctx context.Context, options bubbleTea.RuntimeDashboardOptions)
	WaitForRuntimeExit() (reconfigure bool, err error)
	ShowFatalError(title, message string)
	Close()
}

// activeUnifiedSession holds the shared unified session across configurator/runtime phases.
// It is created by configureContinuous() on first call and reused on reconfigure loops.
var activeUnifiedSession unifiedSessionHandle

// newUnifiedSession creates a new unified session. Replaced in tests.
var newUnifiedSession = func(ctx context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
	return bubbleTea.NewUnifiedSession(ctx, opts)
}

type Configurator struct {
	appMode            AppMode
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
	useContinuousUI    bool
}

type configuratorState int

const (
	configuratorStateModeSelect configuratorState = iota
	configuratorStateClient
	configuratorStateServer
)

func NewConfigurator(
	observer clientConfiguration.Observer,
	selector clientConfiguration.Selector,
	creator clientConfiguration.Creator,
	deleter clientConfiguration.Deleter,
	serverConfigurationManager server.ConfigurationManager,
	selectorFactory selector.Factory,
	textInputFactory text_input.TextInputFactory,
	textAreaFactory text_area.TextAreaFactory,
) *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(
			observer,
			selector,
			deleter,
			creator,
			selectorFactory,
			textInputFactory,
			textAreaFactory,
			clientConfiguration.NewManager(),
		),
		serverConfigurator: newServerConfigurator(serverConfigurationManager, selectorFactory),
		appMode:            NewAppMode(selectorFactory),
		useContinuousUI:    false,
	}
}

func NewDefaultConfigurator(serverConfigurationManager server.ConfigurationManager) *Configurator {
	clientConfResolver := clientConfiguration.NewDefaultResolver()
	uiBundle := uifactory.NewDefaultBundle()
	return NewConfigurator(
		clientConfiguration.NewDefaultObserver(clientConfResolver),
		clientConfiguration.NewDefaultSelector(clientConfResolver),
		clientConfiguration.NewDefaultCreator(clientConfResolver),
		clientConfiguration.NewDefaultDeleter(clientConfResolver),
		serverConfigurationManager,
		uiBundle.SelectorFactory,
		uiBundle.TextInputFactory,
		uiBundle.TextAreaFactory,
	).withContinuousUI()
}

func (p *Configurator) Configure(ctx context.Context) (mode.Mode, error) {
	if p.useContinuousUI {
		return p.configureContinuous(ctx)
	}
	return p.configureFromState(configuratorStateModeSelect)
}

func (p *Configurator) withContinuousUI() *Configurator {
	p.useContinuousUI = true
	return p
}

func (p *Configurator) configureContinuous(ctx context.Context) (mode.Mode, error) {
	if p.clientConfigurator == nil || p.serverConfigurator == nil {
		return mode.Unknown, fmt.Errorf("continuous configurator is not initialized")
	}

	configOpts := bubbleTea.ConfiguratorSessionOptions{
		Observer:            p.clientConfigurator.observer,
		Selector:            p.clientConfigurator.selector,
		Creator:             p.clientConfigurator.creator,
		Deleter:             p.clientConfigurator.deleter,
		ClientConfigManager: p.clientConfigurator.configurationManager,
		ServerConfigManager: p.serverConfigurator.manager,
	}

	if activeUnifiedSession == nil {
		session, err := newUnifiedSession(ctx, configOpts)
		if err != nil {
			return mode.Unknown, err
		}
		activeUnifiedSession = session
	}

	selectedMode, err := activeUnifiedSession.WaitForMode()
	if err != nil {
		if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) {
			activeUnifiedSession.Close()
			activeUnifiedSession = nil
			return mode.Unknown, ErrUserExit
		}
		activeUnifiedSession.Close()
		activeUnifiedSession = nil
		return mode.Unknown, err
	}
	return selectedMode, nil
}

func (p *Configurator) configureFromState(state configuratorState) (mode.Mode, error) {
	selectedMode := mode.Unknown
	for {
		switch state {
		case configuratorStateModeSelect:
			appMode, appModeErr := p.appMode.Mode()
			if appModeErr != nil {
				return mode.Unknown, appModeErr
			}
			selectedMode = appMode
			switch appMode {
			case mode.Client:
				state = configuratorStateClient
			case mode.Server:
				state = configuratorStateServer
			default:
				return mode.Unknown, fmt.Errorf("invalid mode")
			}

		case configuratorStateClient:
			if err := p.clientConfigurator.Configure(); err != nil {
				if errors.Is(err, ErrBackToModeSelection) {
					state = configuratorStateModeSelect
					continue
				}
				return selectedMode, err
			}
			return selectedMode, nil

		case configuratorStateServer:
			if err := p.serverConfigurator.Configure(); err != nil {
				if errors.Is(err, ErrBackToModeSelection) {
					state = configuratorStateModeSelect
					continue
				}
				return selectedMode, err
			}
			return selectedMode, nil

		default:
			return mode.Unknown, fmt.Errorf("unknown configurator state: %d", state)
		}
	}
}
