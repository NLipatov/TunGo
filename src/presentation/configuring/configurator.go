package configuring

import (
	"context"
	"tungo/domain/mode"
)

type Configurator interface {
	Configure(ctx context.Context) (mode.Mode, error)
}
