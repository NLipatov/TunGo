// tungo-e2e provides the HTTP target, Linux agent, and native runner
// used by the tunnel compatibility workflow.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	var err error
	if len(os.Args) < 2 {
		err = fmt.Errorf("usage: tungo-e2e {target|agent|run} [options]")
	} else {
		switch os.Args[1] {
		case "target":
			err = runTarget(ctx, os.Args[2:])
		case "agent":
			err = runAgent(ctx)
		case "run":
			err = runFixture(ctx, os.Args[2:])
		default:
			err = fmt.Errorf("unknown command %q", os.Args[1])
		}
	}
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
