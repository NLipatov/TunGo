package version

import (
	"fmt"
	"strings"
	"tungo/domain/app"
)

// Tag will be set via ldflags by CI release workflow
var Tag = "dev-build"

func Current() string {
	return strings.TrimSpace(Tag)
}

func Run() {
	fmt.Printf("%s %s\n",
		app.Name,
		Current(),
	)
}
