package service

import "testing"

func Test_SignalIs_Unset(t *testing.T) {
	signalByte := byte(0)
	actual := SignalIs(signalByte, Unset)

	if actual != true {
		t.Errorf("Expected %v, got %v", true, actual)
	}
}

func Test_SignalIs_NotUnset(t *testing.T) {
	signalByte := byte(1)
	actual := SignalIs(signalByte, Unset)

	if actual != false {
		t.Errorf("Expected %v, got %v", false, actual)
	}
}

func Test_SignalIs_SessionReset(t *testing.T) {
	signalByte := byte(1)
	actual := SignalIs(signalByte, SessionReset)

	if actual != true {
		t.Errorf("Expected %v, got %v", true, actual)
	}
}

func Test_SignalIs_NotSessionReset(t *testing.T) {
	signalByte := byte(0)
	actual := SignalIs(signalByte, SessionReset)

	if actual != false {
		t.Errorf("Expected %v, got %v", false, actual)
	}
}
