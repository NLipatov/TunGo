package net

import (
	"os/exec"
	"testing"
)

func interfaceExists(ifName string) bool {
	cmd := exec.Command("ip", "link", "show", ifName)
	err := cmd.Run()
	return err == nil
}

func Test(t *testing.T) {
	ifName, err := CreateTun("testtun0")
	if err != nil {
		t.Fatalf("failed to create tunnel: %v", err)
	}

	if !interfaceExists(ifName) {
		t.Errorf("tunnel %s should be created: %s", ifName, err)
		return
	}

	err = DeleteInterface(ifName)
	if err != nil {
		t.Errorf("failed to delete %s tunnel", err)
		return
	}

	if interfaceExists(ifName) {
		t.Errorf("tunnel %s should be deleted: %s", ifName, err)
	}
}
