//go:build windows

package manager

import (
	"fmt"
	"strconv"
	"strings"
)

func routeInterfaceName(ifName string, ifIndex int) (string, error) {
	trimmed := strings.TrimSpace(ifName)
	if trimmed != "" {
		return trimmed, nil
	}
	if ifIndex <= 0 {
		return "", fmt.Errorf("best route returned empty interface name and invalid index %d", ifIndex)
	}
	return strconv.Itoa(ifIndex), nil
}
