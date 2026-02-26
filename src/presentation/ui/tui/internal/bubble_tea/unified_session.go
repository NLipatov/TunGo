package bubble_tea

import (
	"context"
	"errors"
	"sync"

	"tungo/domain/mode"

	tea "charm.land/bubbletea/v2"
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
	phaseFatalError                     // fatal error screen shown, waiting for user dismiss
)

// --- Messages ---

// activateRuntimeMsg is sent via the tea.Program when ActivateRuntime is called.
type activateRuntimeMsg struct {
	ctx     context.Context
	options RuntimeDashboardOptions
}

// fatalErrorMsg transitions the session to a fatal error screen.
type fatalErrorMsg struct {
	message string
}

// --- Events (channel-based coordination with external callers) ---

type unifiedEventKind int

const (
	unifiedEventModeSelected unifiedEventKind = iota
	unifiedEventReconfigure
	unifiedEventRuntimeDisconnected // runtime context canceled (transient network error)
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
	settings     *uiPreferencesProvider
	phase        unifiedPhase
	configurator configuratorSessionModel
	runtime      *RuntimeDashboard
	fatalError   *fatalErrorModel
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
	settings *uiPreferencesProvider,
) (unifiedSessionModel, error) {
	cfgModel, err := newConfiguratorSessionModel(configOpts, settings)
	if err != nil {
		return unifiedSessionModel{}, err
	}
	return unifiedSessionModel{
		settings:     settings,
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
		rt := NewRuntimeDashboard(msg.ctx, msg.options, m.settings)
		rt.runtimeSeq = m.runtimeSeq
		if m.width > 0 || m.height > 0 {
			rt.width = m.width
			rt.height = m.height
		}
		m.runtime = &rt
		m.phase = phaseRuntime
		return m, m.runtime.Init()

	case runtimeContextDoneMsg:
		// Runtime context was canceled (e.g. network disconnection).
		// Transition back to waiting so the next ActivateRuntime can reuse the session.
		// Check seq to ignore stale messages from a previous runtime.
		if m.phase == phaseRuntime && msg.seq == m.runtimeSeq {
			m.stopAllLogWaits()
			GlobalRuntimeLogWriteSeparator("disconnected")
			m.runtime = nil
			m.phase = phaseWaitingForRuntime
			m.sendEvent(unifiedEvent{kind: unifiedEventRuntimeDisconnected})
			return m, nil
		}
		return m, nil

	case fatalErrorMsg:
		m.stopAllLogWaits()
		fe := newFatalErrorModel(msg.message, m.settings)
		fe.width = m.width
		fe.height = m.height
		m.fatalError = &fe
		m.phase = phaseFatalError
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
	case phaseFatalError:
		return m.updateFatalError(msg)
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
		GlobalRuntimeLogWriteSeparator("reconfigured")
		// Reset configurator for a fresh cycle.
		newCfg, err := newConfiguratorSessionModel(m.configOpts, m.settings)
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

func (m unifiedSessionModel) updateFatalError(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.fatalError == nil {
		return m, nil
	}
	updated, cmd := m.fatalError.Update(msg)
	fe := updated.(fatalErrorModel)
	m.fatalError = &fe

	if fe.dismissed {
		m.sendEvent(unifiedEvent{kind: unifiedEventExit})
		return m, tea.Quit
	}

	cmd = filterQuit(cmd)
	return m, cmd
}

func (m unifiedSessionModel) altScreenView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m unifiedSessionModel) View() tea.View {
	switch m.phase {
	case phaseConfiguring:
		return m.configurator.View()
	case phaseWaitingForRuntime:
		return m.altScreenView(m.waitingView())
	case phaseRuntime:
		if m.runtime != nil {
			return m.runtime.View()
		}
		return m.altScreenView(m.waitingView())
	case phaseFatalError:
		if m.fatalError != nil {
			return m.fatalError.View()
		}
		return m.altScreenView("")
	}
	return m.altScreenView("")
}

func (m unifiedSessionModel) waitingView() string {
	prefs := m.settings.Preferences()
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

// contextDoneChan returns ctx.Done() if ctx is non-nil, or nil otherwise.
// A nil channel in select is never selected, so this is safe to use in
// select cases when the context may not be set.
func contextDoneChan(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
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
	program   *tea.Program
	events    chan unifiedEvent
	done      chan struct{}
	closeOnce sync.Once
	err       error
	appCtx    context.Context
}

var newUnifiedSessionProgram = func(model tea.Model) *tea.Program {
	return tea.NewProgram(model)
}

// NewUnifiedSession creates and starts a unified session.
func NewUnifiedSession(appCtx context.Context, configOpts ConfiguratorSessionOptions) (*UnifiedSession, error) {
	events := make(chan unifiedEvent, 4)
	settings := loadUISettingsFromDisk()
	model, err := newUnifiedSessionModel(appCtx, configOpts, events, settings)
	if err != nil {
		return nil, err
	}

	program := newUnifiedSessionProgram(model)
	session := &UnifiedSession{
		program: program,
		events:  events,
		done:    make(chan struct{}),
		appCtx:  appCtx,
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
	appCtxDone := contextDoneChan(s.appCtx)
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
			if m, err := s.drainModeEvent(); m != mode.Unknown || err != nil {
				return m, err
			}
			if s.err != nil {
				return mode.Unknown, s.err
			}
			return mode.Unknown, ErrUnifiedSessionClosed
		case <-appCtxDone:
			return mode.Unknown, ErrUnifiedSessionQuit
		}
	}
}

// ActivateRuntime transitions the unified session to runtime dashboard phase.
func (s *UnifiedSession) ActivateRuntime(ctx context.Context, options RuntimeDashboardOptions) {
	s.program.Send(activateRuntimeMsg{ctx: ctx, options: options})
}

// WaitForRuntimeExit blocks until the runtime phase ends (reconfigure or exit).
func (s *UnifiedSession) WaitForRuntimeExit() (reconfigure bool, err error) {
	appCtxDone := contextDoneChan(s.appCtx)
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
			if r, err := s.drainRuntimeEvent(); r || err != nil {
				return r, err
			}
			if s.err != nil {
				return false, s.err
			}
			return false, ErrUnifiedSessionClosed
		case <-appCtxDone:
			return false, ErrUnifiedSessionQuit
		}
	}
}

// drainModeEvent reads any buffered events that arrived before the done
// channel was selected. This resolves the race where the model sends an event
// and the program exits at the same time — Go's select may pick done first.
// Returns (mode.Unknown, nil) when no relevant event was buffered.
func (s *UnifiedSession) drainModeEvent() (mode.Mode, error) {
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				return mode.Unknown, nil
			}
			switch event.kind {
			case unifiedEventModeSelected:
				return event.mode, nil
			case unifiedEventExit:
				return mode.Unknown, ErrUnifiedSessionQuit
			case unifiedEventError:
				return mode.Unknown, event.err
			default:
				continue
			}
		default:
			return mode.Unknown, nil
		}
	}
}

// drainRuntimeEvent reads any buffered events that arrived before the done
// channel was selected. Returns (false, nil) when no relevant event was buffered.
func (s *UnifiedSession) drainRuntimeEvent() (reconfigure bool, err error) {
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				return false, nil
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
				return true, nil
			default:
				continue
			}
		default:
			return false, nil
		}
	}
}

// ShowFatalError transitions the session to a fatal error screen and blocks
// until the user dismisses it (Enter / Esc / q) or the program exits.
func (s *UnifiedSession) ShowFatalError(message string) {
	go s.program.Send(fatalErrorMsg{message: message})
	select {
	case <-s.done:
	case <-contextDoneChan(s.appCtx):
	}
}

// Close gracefully stops the unified session program.
func (s *UnifiedSession) Close() {
	s.closeOnce.Do(func() {
		go s.program.Quit()
		select {
		case <-s.done:
			clearTerminalAfterTUI()
		case <-contextDoneChan(s.appCtx):
		}
	})
}

// Done returns a channel that is closed when the program exits.
func (s *UnifiedSession) Done() <-chan struct{} {
	return s.done
}
