package tui

import "fmt"

type RuntimeUIResult struct {
	UserQuit bool
	Err      error
}

func resolveRuntimeEnd(
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
		if workerErr != nil {
			return workerErr
		}
		if userExit {
			return nil
		}
		return fmt.Errorf("runtime UI failed: %w", uiResult.Err)
	}
	if uiResult.UserQuit {
		if workerErr != nil {
			return workerErr
		}
		return errReconfigureRequested
	}
	return workerErr
}
