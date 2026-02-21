package version

import (
	"context"
	"fmt"
	"strings"
	"tungo/domain/app"
)

// Tag will be set via ldflags by CI release workflow
var Tag = "dev-build"

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func Current() string {
	return strings.TrimSpace(Tag)
}

func (r *Runner) Run(_ context.Context) {
	fmt.Printf("%s %s\n",
		app.Name,
		Current(),
	)
}
