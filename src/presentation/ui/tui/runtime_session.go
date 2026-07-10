package tui

import (
	"context"
	"errors"
	"fmt"
)

type RuntimeUIResult struct {
	UserQuit bool
	Err      error
}

func resolveRuntimeSessionEnd(
	uiResult RuntimeUIResult,
	workerErr error,
	isUserExit func(error) bool,
	onRuntimeUIError func(error),
) error {
	if isUserExit == nil {
		isUserExit = func(error) bool { return false }
	}

	if uiResult.Err != nil {
		userExit := isUserExit(uiResult.Err)
		if !userExit && onRuntimeUIError != nil {
			onRuntimeUIError(uiResult.Err)
		}
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return workerErr
		}
		if userExit {
			return context.Canceled
		}
		return fmt.Errorf("runtime UI failed: %w", uiResult.Err)
	}
	if uiResult.UserQuit {
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return workerErr
		}
		return errReconfigureRequested
	}
	return workerErr
}
