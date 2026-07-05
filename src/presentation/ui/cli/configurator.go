package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"tungo/domain/app"
	"tungo/runtime"
)

const (
	serverModeArg = "s"
	clientModeArg = "c"
)

func Configure(_ context.Context) (runtime.Mode, error) {
	switch strings.Join(trimArgs(os.Args[1:]), " ") {
	case clientModeArg:
		return runtime.ModeClient, nil
	case serverModeArg:
		return runtime.ModeServer, nil
	default:
		printUsage()
		return 0, fmt.Errorf("invalid arguments")
	}
}

func trimArgs(args []string) []string {
	for i, v := range args {
		args[i] = strings.TrimSpace(v)
	}

	return args
}

func printUsage() {
	fmt.Printf(`Usage: %s <mode>
Modes:
  %s  - Server
  %s  - Client
`, app.Name, serverModeArg, clientModeArg)
}
