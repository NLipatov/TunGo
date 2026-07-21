package client

import "path/filepath"

type DefaultObserver struct {
	resolver Resolver
}

func NewDefaultObserver(resolver Resolver) *DefaultObserver {
	return &DefaultObserver{
		resolver: resolver,
	}
}

func (o *DefaultObserver) Observe() ([]string, error) {
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
