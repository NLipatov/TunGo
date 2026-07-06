package configuration

import (
	"fmt"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/platform"
	"tungo/infrastructure/PAL/stat"
)

func NewDefaultControls() (Controls, error) {
	clientResolver := clientConfiguration.NewDefaultResolver()
	controls := Controls{
		Client: &clientControl{
			observer: clientConfiguration.NewDefaultObserver(clientResolver),
			selector: clientConfiguration.NewDefaultSelector(clientResolver),
			creator:  clientConfiguration.NewDefaultCreator(clientResolver),
			deleter:  clientConfiguration.NewDefaultDeleter(clientResolver),
			manager:  clientConfiguration.NewManager(),
		},
	}

	if !platform.Capabilities().ServerModeSupported() {
		return controls, nil
	}

	serverResolver := serverConfiguration.NewServerResolver()
	serverManager, err := serverConfiguration.NewManager(serverResolver, stat.NewDefaultStat())
	if err != nil {
		return Controls{}, fmt.Errorf("configuration error: %w", err)
	}
	controls.Server = &serverControl{manager: serverManager}
	return controls, nil
}
