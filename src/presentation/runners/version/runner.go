package version

import (
	"context"
	"fmt"
	"tungo/domain/app"
)

// CI release workflow is setting this variable via ldflags
var VersionTag = "version not set"

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Run(_ context.Context) {
	fmt.Printf("%s %s\n",
		app.Name,
		VersionTag,
	)
}
