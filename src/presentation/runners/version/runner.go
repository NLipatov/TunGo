package version

import (
	"context"
	"fmt"
	"strings"
	"tungo/domain/app"
)

// Tag will be set via ldflags by CI release workflow
var Tag = "version not set"

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Run(_ context.Context) {
	fmt.Printf("%s %s\n",
		app.Name,
		displayTag(Tag),
	)
}

func displayTag(tag string) string {
	normalized := strings.TrimSpace(tag)
	if normalized == "" || strings.EqualFold(normalized, "version not set") {
		return "dev-build"
	}
	return normalized
}
