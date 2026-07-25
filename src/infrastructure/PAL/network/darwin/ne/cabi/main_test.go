//go:build darwin || ios

package main

import (
	"runtime/cgo"
	"testing"

	control "tungo/infrastructure/PAL/network/darwin/ne/internal/controller"
)

func TestControllerHandleLifecycle(t *testing.T) {
	handle := cgo.NewHandle(control.New())

	controller, err := resolveController(handle)
	if err != nil {
		t.Fatalf("resolve controller: %v", err)
	}
	if controller == nil {
		t.Fatal("resolved controller is nil")
	}

	releaseControllerHandle(handle)
	if _, err := resolveController(handle); err == nil {
		t.Fatal("released controller handle remains valid")
	}

	releaseControllerHandle(handle)
}

func TestResolveControllerRejectsZeroHandle(t *testing.T) {
	if _, err := resolveController(0); err == nil {
		t.Fatal("zero controller handle is accepted")
	}
}
