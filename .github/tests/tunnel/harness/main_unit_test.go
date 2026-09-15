package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRejectsOutsideGitHubActions(t *testing.T) {
	// Even valid arguments must stop before privileges, network changes or SSH.
	t.Setenv("GITHUB_ACTIONS", "false")
	for _, command := range []string{"run", "client"} {
		err := run(context.Background(), runInfo{
			command: command, protocol: "UDP", workdir: "unused",
		})
		if err == nil || !strings.Contains(err.Error(), "disposable GitHub Actions machines") {
			t.Fatalf("%s: expected refusal outside GitHub Actions, got %v", command, err)
		}
	}
}
