package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCommandExitHelperProcess")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", code),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit error for code %d", code)
	}
	return err
}

func TestCommandExitHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	if err != nil {
		os.Exit(2)
	}
	os.Exit(code)
}
