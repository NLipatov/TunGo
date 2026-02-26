package tui

import (
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

var ErrUserExit = errors.New("user requested tui exit")
var ErrBackToModeSelection = errors.New("back to mode selection requested")
var ErrSessionClosed = bubbleTea.ErrUnifiedSessionClosed
