package logging

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

func SetCrashOutput(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		_, _ = fmt.Fprintf(f, "\n--- crash at %s ---\n\n", time.Now().Format(time.RFC3339))
	}
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
}
