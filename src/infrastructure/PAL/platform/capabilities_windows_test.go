package platform

import "testing"

func TestServerModeSupported_Windows(t *testing.T) {
	if Capabilities().ServerModeSupported() {
		t.Fatal("expected ServerModeSupported() == false on windows")
	}
}
