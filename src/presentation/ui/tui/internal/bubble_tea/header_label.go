package bubble_tea

import (
	"strings"
	"sync"
	"tungo/presentation/runners/version"
)

const defaultProductLabel = "TunGo"
const devBuildLabel = "dev-build"

var (
	productLabelOnce  sync.Once
	productLabelValue string
)

func productLabel() string {
	productLabelOnce.Do(func() {
		productLabelValue = resolveProductLabel()
	})
	return productLabelValue
}

func resolveProductLabel() string {
	tag := strings.TrimSpace(version.Tag)
	if tag == "" || strings.EqualFold(tag, "version not set") {
		return defaultProductLabel + " [" + devBuildLabel + "]"
	}
	return defaultProductLabel + " [" + tag + "]"
}

func resetProductLabel() {
	productLabelOnce = sync.Once{}
	productLabelValue = ""
}
