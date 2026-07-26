package tui

import (
	"context"
	"errors"

	"tungo/application/runtime"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

func (t *TUI) configure(ctx context.Context) (runtime.Mode, error) {
	if t.session == nil {
		session, err := bubbleTea.NewUnifiedSession(ctx, t.sessionOptions)
		if err != nil {
			return 0, err
		}
		t.session = session
	}

	selectedMode, err := t.session.WaitForMode()
	if err != nil {
		if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
			t.closeSession()
			return 0, ErrUserExit
		}
		t.closeSession()
		return 0, err
	}
	return selectedMode, nil
}

// closeSession closes the active unified session.
func (t *TUI) closeSession() {
	if t.session != nil {
		t.session.Close()
		t.session = nil
	}
}

// Close releases the active unified session. Safe to call multiple times.
func (t *TUI) Close() {
	t.closeSession()
}
