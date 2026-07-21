//go:build darwin || ios

package apple

import (
	"testing"
)

func TestControllerStartRejectsInvalidDescriptor(t *testing.T) {
	controller := NewController()
	if _, err := controller.Start(-1); err == nil {
		t.Fatal("expected invalid descriptor error")
	}
}

func TestControllerRejectsUnknownHandle(t *testing.T) {
	controller := NewController()
	if err := controller.Stop(42); err == nil {
		t.Fatal("expected unknown handle error")
	}
	if err := controller.Pause(42); err == nil {
		t.Fatal("expected unknown handle error")
	}
	if err := controller.Restart(42); err == nil {
		t.Fatal("expected unknown handle error")
	}
	if _, err := controller.Status(42); err == nil {
		t.Fatal("expected unknown handle error")
	}
}
