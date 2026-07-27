package tui

import (
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

var ErrUserExit = errors.New("user requested tui exit")
var ErrSessionClosed = bubbleTea.ErrUnifiedSessionClosed
var errReconfigureRequested = errors.New("reconfigure requested")
