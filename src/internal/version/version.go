package version

import "strings"

// Tag will be set via ldflags by CI release workflow
var Tag = "dev-build"

func Current() string {
	return strings.TrimSpace(Tag)
}
