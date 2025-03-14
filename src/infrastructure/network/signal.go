package network

type Signal int

const (
	Unset Signal = iota
	SessionReset
)

func SignalIs(signal byte, networkSignal Signal) bool {
	return Signal(signal) == networkSignal
}
