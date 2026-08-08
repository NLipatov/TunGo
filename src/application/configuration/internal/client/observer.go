package client

import "path/filepath"

type Observer struct {
	resolver Resolver
}

func NewObserver(resolver Resolver) *Observer {
	return &Observer{
		resolver: resolver,
	}
}

func (o *Observer) Observe() ([]string, error) {
	defaultConfPath, defaultConfPathErr := o.resolver.Resolve()
	if defaultConfPathErr != nil {
		return nil, defaultConfPathErr
	}

	dir := filepath.Dir(defaultConfPath)
	defaultBase := filepath.Base(defaultConfPath)
	pattern := filepath.Join(dir, defaultBase+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, match := range matches {
		if match == defaultConfPath {
			continue
		}
		results = append(results, match)
	}

	return results, nil
}
