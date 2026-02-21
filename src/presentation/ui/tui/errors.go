package tui

import "errors"

var ErrUserExit = errors.New("user requested tui exit")
var ErrBackToModeSelection = errors.New("back to mode selection requested")
