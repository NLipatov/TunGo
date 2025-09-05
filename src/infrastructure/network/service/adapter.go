package service

import (
	"tungo/application"
	"tungo/application/network/ip"
	nip "tungo/infrastructure/network/ip"
)

// Adapter is used to detect and handle service frames
type Adapter struct {
	adapter      application.ConnectionAdapter
	headerParser ip.HeaderParser
	detector     application.DocNetDetector
}

func NewDefaultAdapter(
	adapter application.ConnectionAdapter,
) *Adapter {
	return &Adapter{
		adapter:      adapter,
		headerParser: nip.NewHeaderParser(),
		detector:     NewDocNetDetector(),
	}
}

func NewAdapter(
	adapter application.ConnectionAdapter,
	headerParser ip.HeaderParser,
	detector application.DocNetDetector,
) *Adapter {
	return &Adapter{
		adapter:      adapter,
		headerParser: headerParser,
		detector:     detector,
	}
}

func (a *Adapter) Write(data []byte) (int, error) {
	return a.adapter.Write(data)
}

func (a *Adapter) Read(buffer []byte) (int, error) {
	n, err := a.adapter.Read(buffer)
	if err != nil {
		return n, err
	}
	addr, addrErr := a.headerParser.DestinationAddress(buffer[:n])
	if addrErr != nil {
		return n, addrErr
	}
	if a.detector.IsInDocNet(addr) {
		// handle service frame
	}
	return n, nil
}

func (a *Adapter) Close() error {
	return a.adapter.Close()
}
