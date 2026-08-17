//go:build windows

package resume

import (
	"context"
)

// Watch is a no-op on Windows.
func Watch(ctx context.Context) <-chan struct{} {
	return make(chan struct{})
}
