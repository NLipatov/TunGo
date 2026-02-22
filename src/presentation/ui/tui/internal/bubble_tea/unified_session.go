package bubble_tea

import (
	"context"
	"errors"
	"sync"

	"tungo/domain/mode"

	tea "github.com/charmbracelet/bubbletea"
)

// Errors returned by the unified session.
var (
	ErrUnifiedSessionClosed              = errors.New("unified session closed")
	ErrUnifiedSessionQuit                = errors.New("unified session user quit")
	ErrUnifiedSessionRuntimeDisconnected = errors.New("unified session runtime disconnected")
)

// unifiedPhase represents the current phase of the unified session.
type unifiedPhase int

const (
	phaseConfiguring       unifiedPhase = iota
	phaseWaitingForRuntime              // configurator done, waiting for ActivateRuntime call
	phaseRuntime                        // runtime dashboard is active
)

// --- Messages ---

// modeSelectedMsg is sent internally when the configurator completes.
type modeSelectedMsg struct {
	mode mode.Mode
}

// activateRuntimeMsg is sent via the tea.Program when ActivateRuntime is called.
type activateRuntimeMsg struct {
	ctx     context.Context
	options RuntimeDashboardOptions
}

// runtimeDoneMsg is sent when the runtime sub-model signals completion.
type runtimeDoneMsg struct {
	reconfigure bool
	exit        bool
}

// --- Events (channel-based coordination with external callers) ---

type unifiedEventKind int

const (
	unifiedEventModeSelected       unifiedEventKind = iota
	unifiedEventReconfigure
	unifiedEventRuntimeDisconnected // runtime context cancelled (transient network error)
	unifiedEventExit
	unifiedEventError
)

type unifiedEvent struct {
	kind unifiedEventKind
	mode mode.Mode
	err  error
}

// --- Unified session model ---

type unifiedSessionModel struct {
	phase        unifiedPhase
	configurator configuratorSessionModel
	runtime      *RuntimeDashboard
	configOpts   ConfiguratorSessionOptions
	width        int
	height       int
	events       chan<- unifiedEvent
	appCtx       context.Context
	runtimeSeq   uint64
}

func newUnifiedSessionModel(
	appCtx context.Context,
	configOpts ConfiguratorSessionOptions,
	events chan<- unifiedEvent,
) (unifiedSessionModel, error) {
	cfgModel, err := newConfiguratorSessionModel(configOpts)
	if err != nil {
		return unifiedSessionModel{}, err
	}
	return unifiedSessionModel{
		phase:        phaseConfiguring,
		configurator: cfgModel,
		configOpts:   configOpts,
		events:       events,
		appCtx:       appCtx,
	}, nil
}

func (m unifiedSessionModel) Init() tea.Cmd {
	return tea.Batch(
		m.configurator.Init(),
		waitForContextDone(m.appCtx),
	)
}

func (m unifiedSessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.delegateToActive(msg)

	case contextDoneMsg:
		m.stopAllLogWaits()
		m.sendEvent(unifiedEvent{kind: unifiedEventExit})
		return m, tea.Quit

	case activateRuntimeMsg:
		if m.phase != phaseWaitingForRuntime {
			return m, nil
		}
		m.runtimeSeq++
		rt := NewRuntimeDashboard(msg.ctx, msg.options)
		rt.runtimeSeq = m.runtimeSeq
		if m.width > 0 || m.height > 0 {
			rt.width = m.width
			rt.height = m.height
		}
		m.runtime = &rt
		m.phase = phaseRuntime
		return m, m.runtime.Init()

	case runtimeContextDoneMsg:
		// Runtime context was cancelled (e.g. network disconnection).
		// Transition back to waiting so the next ActivateRuntime can reuse the session.
		// Check seq to ignore stale messages from a previous runtime.
		if m.phase == phaseRuntime && msg.seq == m.runtimeSeq {
			m.stopAllLogWaits()
			m.runtime = nil
			m.phase = phaseWaitingForRuntime
			m.sendEvent(unifiedEvent{kind: unifiedEventRuntimeDisconnected})
			return m, nil
		}
		return m, nil
	}

	return m.delegateToActive(msg)
}

func (m unifiedSessionModel) delegateToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseConfiguring:
		return m.updateConfigurator(msg)
	case phaseWaitingForRuntime:
		// Ignore sub-model messages while waiting.
		return m, nil
	case phaseRuntime:
		return m.updateRuntime(msg)
	}
	return m, nil
}

func (m unifiedSessionModel) updateConfigurator(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.configurator.Update(msg)
	cfgModel := updated.(configuratorSessionModel)
	m.configurator = cfgModel

	if cfgModel.done {
		if cfgModel.resultErr != nil {
			if errors.Is(cfgModel.resultErr, ErrConfiguratorSessionUserExit) {
				m.sendEvent(unifiedEvent{kind: unifiedEventExit})
				return m, tea.Quit
			}
			m.sendEvent(unifiedEvent{kind: unifiedEventError, err: cfgModel.resultErr})
			return m, tea.Quit
		}
		// Mode selected — transition to waiting phase.
		m.phase = phaseWaitingForRuntime
		m.sendEvent(unifiedEvent{kind: unifiedEventModeSelected, mode: cfgModel.resultMode})
		return m, nil
	}

	// Filter out tea.Quit from configurator (it uses Quit for "done").
	cmd = filterQuit(cmd)
	return m, cmd
}

func (m unifiedSessionModel) updateRuntime(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.runtime == nil {
		return m, nil
	}
	updated, cmd := m.runtime.Update(msg)
	rtModel := updated.(RuntimeDashboard)
	m.runtime = &rtModel

	if rtModel.exitRequested {
		m.stopAllLogWaits()
		m.sendEvent(unifiedEvent{kind: unifiedEventExit})
		return m, tea.Quit
	}
	if rtModel.reconfigureRequested {
		// Reset configurator for a fresh cycle.
		newCfg, err := newConfiguratorSessionModel(m.configOpts)
		if err != nil {
			m.sendEvent(unifiedEvent{kind: unifiedEventError, err: err})
			return m, tea.Quit
		}
		if m.width > 0 || m.height > 0 {
			newCfg.width = m.width
			newCfg.height = m.height
		}
		m.configurator = newCfg
		m.runtime = nil
		m.phase = phaseConfiguring
		m.sendEvent(unifiedEvent{kind: unifiedEventReconfigure})
		return m, tea.Batch(
			m.configurator.Init(),
			tea.ClearScreen,
		)
	}

	// Filter out tea.Quit from runtime (it uses Quit for both exit and reconfigure).
	cmd = filterQuit(cmd)
	return m, cmd
}

func (m unifiedSessionModel) View() string {
	switch m.phase {
	case phaseConfiguring:
		return m.configurator.View()
	case phaseWaitingForRuntime:
		return m.waitingView()
	case phaseRuntime:
		if m.runtime != nil {
			return m.runtime.View()
		}
		return m.waitingView()
	}
	return ""
}

func (m unifiedSessionModel) waitingView() string {
	prefs := CurrentUIPreferences()
	styles := resolveUIStyles(prefs)
	body := []string{"Starting..."}
	return renderScreen(
		m.width,
		m.height,
		renderTabsLine(productLabel(), "configurator", selectorTabs[:], 0, contentWidthForTerminal(m.width), prefs.Theme, styles),
		"",
		body,
		"",
		prefs,
		styles,
	)
}

func (m *unifiedSessionModel) stopAllLogWaits() {
	m.configurator.stopLogWait()
	if m.runtime != nil {
		m.runtime.stopLogWait()
	}
}

func (m unifiedSessionModel) sendEvent(event unifiedEvent) {
	m.events <- event
}

// --- Context done message ---

type contextDoneMsg struct{}

func waitForContextDone(ctx context.Context) tea.Cmd {
	if ctx == nil {
		return nil
	}
	return func() tea.Msg {
		<-ctx.Done()
		return contextDoneMsg{}
	}
}

// filterQuit wraps a tea.Cmd so that any tea.QuitMsg it produces is
// silently swallowed. Also handles tea.BatchMsg by recursively filtering
// each sub-command. The command still runs asynchronously via the
// Bubble Tea runtime — we never call cmd() on the main goroutine.
func filterQuit(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			return nil
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			filtered := make(tea.BatchMsg, len(batch))
			for i, sub := range batch {
				filtered[i] = filterQuit(sub)
			}
			return filtered
		}
		return msg
	}
}

// --- UnifiedSession (external handle) ---

// UnifiedSession manages a single tea.Program across configurator and runtime phases.
type UnifiedSession struct {
	program *tea.Program
	events  chan unifiedEvent
	done    chan struct{}
	closeOnce sync.Once
	err     error
}

var newUnifiedSessionProgram = func(model tea.Model) *tea.Program {
	return tea.NewProgram(model, tea.WithAltScreen())
}

// NewUnifiedSession creates and starts a unified session.
func NewUnifiedSession(appCtx context.Context, configOpts ConfiguratorSessionOptions) (*UnifiedSession, error) {
	events := make(chan unifiedEvent, 4)
	model, err := newUnifiedSessionModel(appCtx, configOpts, events)
	if err != nil {
		return nil, err
	}

	program := newUnifiedSessionProgram(model)
	session := &UnifiedSession{
		program: program,
		events:  events,
		done:    make(chan struct{}),
	}

	go func() {
		defer close(session.done)
		finalModel, runErr := program.Run()
		if runErr != nil {
			session.err = runErr
		}
		if m, ok := finalModel.(unifiedSessionModel); ok {
			m.stopAllLogWaits()
		}
	}()

	return session, nil
}

// WaitForMode blocks until the user selects a mode in the configurator phase.
func (s *UnifiedSession) WaitForMode() (mode.Mode, error) {
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				return mode.Unknown, ErrUnifiedSessionClosed
			}
			switch event.kind {
			case unifiedEventModeSelected:
				return event.mode, nil
			case unifiedEventExit:
				return mode.Unknown, ErrUnifiedSessionQuit
			case unifiedEventError:
				return mode.Unknown, event.err
			case unifiedEventReconfigure:
				// Shouldn't happen during WaitForMode, but continue listening.
				continue
			}
		case <-s.done:
			if s.err != nil {
				return mode.Unknown, s.err
			}
			return mode.Unknown, ErrUnifiedSessionClosed
		}
	}
}

// ActivateRuntime transitions the unified session to runtime dashboard phase.
func (s *UnifiedSession) ActivateRuntime(ctx context.Context, options RuntimeDashboardOptions) {
	s.program.Send(activateRuntimeMsg{ctx: ctx, options: options})
}

// WaitForRuntimeExit blocks until the runtime phase ends (reconfigure or exit).
func (s *UnifiedSession) WaitForRuntimeExit() (reconfigure bool, err error) {
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				return false, ErrUnifiedSessionClosed
			}
			switch event.kind {
			case unifiedEventReconfigure:
				return true, nil
			case unifiedEventRuntimeDisconnected:
				return false, ErrUnifiedSessionRuntimeDisconnected
			case unifiedEventExit:
				return false, ErrUnifiedSessionQuit
			case unifiedEventError:
				return false, event.err
			case unifiedEventModeSelected:
				// Shouldn't happen during WaitForRuntimeExit, but treat as reconfigure.
				return true, nil
			}
		case <-s.done:
			if s.err != nil {
				return false, s.err
			}
			return false, ErrUnifiedSessionClosed
		}
	}
}

// Close gracefully stops the unified session program.
func (s *UnifiedSession) Close() {
	s.closeOnce.Do(func() {
		s.program.Quit()
		<-s.done
		clearTerminalAfterTUI()
	})
}

// Done returns a channel that is closed when the program exits.
func (s *UnifiedSession) Done() <-chan struct{} {
	return s.done
}
