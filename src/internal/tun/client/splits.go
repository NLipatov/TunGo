package client

import "slices"

func withoutRoutes(splits []string, excluded ...string) []string {
	routes := make([]string, 0, len(splits))
	for _, split := range splits {
		if !slices.Contains(excluded, split) {
			routes = append(routes, split)
		}
	}
	return routes
}
