package tui

import (
	"errors"
)

var ErrUserExit = errors.New("user requested tui exit")
var errReconfigureRequested = errors.New("reconfigure requested")
