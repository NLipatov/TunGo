package version

import (
	"context"
	"fmt"
	"tungo/domain/app"
)

const (
	VersionTag = "0.0.0"
)

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Run(_ context.Context) {
	fmt.Printf("%s v%s\n",
		app.Name,
		VersionTag,
	)
}
