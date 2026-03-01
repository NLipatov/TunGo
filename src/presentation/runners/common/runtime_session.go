package common

import (
	"context"
	"errors"
	"fmt"
)

type RuntimeUIResult struct {
	UserQuit bool
	Err      error
}

func WaitForRuntimeSessionEnd(
	cancel context.CancelFunc,
	uiResultCh <-chan RuntimeUIResult,
	workerErrCh <-chan error,
	isUserExit func(error) bool,
	onRuntimeUIError func(error),
) error {
	if cancel == nil {
		cancel = func() {}
	}
	if isUserExit == nil {
		isUserExit = func(error) bool { return false }
	}

	for {
		select {
		case workerErr := <-workerErrCh:
			cancel()
			uiResult := <-uiResultCh
			if uiResult.Err != nil && !isUserExit(uiResult.Err) && onRuntimeUIError != nil {
				onRuntimeUIError(uiResult.Err)
			}
			return workerErr
		case uiResult := <-uiResultCh:
			if uiResult.Err != nil {
				cancel()
				workerErr := <-workerErrCh
				if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
					return workerErr
				}
				if isUserExit(uiResult.Err) {
					return context.Canceled
				}
				return fmt.Errorf("runtime UI failed: %w", uiResult.Err)
			}
			if uiResult.UserQuit {
				cancel()
				workerErr := <-workerErrCh
				if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
					return workerErr
				}
				return ErrReconfigureRequested
			}
			cancel()
			return <-workerErrCh
		}
	}
}
