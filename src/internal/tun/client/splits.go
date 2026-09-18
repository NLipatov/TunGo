//go:build linux || darwin

package client

func withoutTunSubnet(splits []string, tunSubnet string) []string {
	routes := make([]string, 0, len(splits))
	for _, split := range splits {
		if split != tunSubnet {
			routes = append(routes, split)
		}
	}
	return routes
}
