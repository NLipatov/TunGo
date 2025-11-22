package client

import (
	"strings"
	"tungo/infrastructure/PAL/args"
)

const (
	configFlag   = "--config"
	configFlagEq = "--config="
)

type ArgumentResolver struct {
	resolver     Resolver
	argsProvider args.Provider
}

func NewArgumentResolver(resolver Resolver, argsProvider args.Provider) Resolver {
	return &ArgumentResolver{
		resolver:     resolver,
		argsProvider: argsProvider,
	}
}

func (a *ArgumentResolver) Resolve() (string, error) {
	if path, ok := a.configPathArgument(); ok {
		return path, nil
	}
	return a.resolver.Resolve()
}

func (a *ArgumentResolver) configPathArgument() (string, bool) {
	arguments := a.argsProvider.Args()
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		// case: --config=/path/to/file
		if strings.HasPrefix(arg, configFlagEq) {
			path := arg[len(configFlagEq):]
			if path != "" {
				return path, true
			}
			return "", false
		}
		// case: --config /path/to/file
		if arg == configFlag && i+1 < len(arguments) {
			path := arguments[i+1]
			if path != "" && !strings.HasPrefix(path, "-") {
				return path, true
			}
			return "", false
		}
	}

	return "", false
}
