package bubble_tea

import (
	"strings"
	"tungo/presentation/runners/version"
)

const defaultProductLabel = "TunGo"
const devBuildLabel = "dev-build"

func productLabel() string {
	tag := strings.TrimSpace(version.Tag)
	if tag == "" || strings.EqualFold(tag, "version not set") {
		return defaultProductLabel + " [" + devBuildLabel + "]"
	}
	return defaultProductLabel + " [" + tag + "]"
}
