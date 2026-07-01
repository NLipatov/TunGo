package configuring

import (
	"context"
	"tungo/domain/command"
)

type Configurator interface {
	Configure(ctx context.Context) (command.Command, error)
}
