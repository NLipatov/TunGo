package runtime

import (
	"fmt"
	runtimeClient "tungo/runtime/internal/client"
	runtimeServer "tungo/runtime/internal/server"
)

// New constructs a runtime without starting it.
func New(mode Mode) (Runtime, error) {
	switch mode {
	case ModeServer:
		resolver, manager, err := runtimeServer.NewDefaultConfiguration()
		if err != nil {
			return nil, err
		}
		return runtimeServer.NewRuntime(resolver, manager)
	case ModeClient:
		return runtimeClient.NewRuntime()
	default:
		return nil, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}
