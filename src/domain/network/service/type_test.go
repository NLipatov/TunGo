package service

import "testing"

func Test_TypeIs_Unset(t *testing.T) {
	signalByte := byte(0)
	actual := PacketTypeIs(signalByte, Unknown)

	if actual != true {
		t.Errorf("Expected %v, got %v", true, actual)
	}
}

func Test_TypeIs_NotUnset(t *testing.T) {
	signalByte := byte(1)
	actual := PacketTypeIs(signalByte, Unknown)

	if actual != false {
		t.Errorf("Expected %v, got %v", false, actual)
	}
}

func Test_TypeIs_SessionReset(t *testing.T) {
	signalByte := byte(1)
	actual := PacketTypeIs(signalByte, SessionReset)

	if actual != true {
		t.Errorf("Expected %v, got %v", true, actual)
	}
}

func Test_TypeIs_NotSessionReset(t *testing.T) {
	signalByte := byte(0)
	actual := PacketTypeIs(signalByte, SessionReset)

	if actual != false {
		t.Errorf("Expected %v, got %v", false, actual)
	}
}
