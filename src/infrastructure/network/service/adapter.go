package service

import (
	"tungo/application"
	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
	nip "tungo/infrastructure/network/ip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Adapter is used to detect and handle service frames
type Adapter struct {
	adapter         application.ConnectionAdapter
	headerParser    ip.HeaderParser
	serviceFrame    *Frame
	mtuProbeHandler Handler
}

func NewDefaultAdapter(
	adapter application.ConnectionAdapter,
) *Adapter {
	return &Adapter{
		adapter:      adapter, // TUN device adapter
		headerParser: nip.NewHeaderParser(),
		serviceFrame: NewDefaultFrame(),
	}
}

func NewAdapter(
	adapter application.ConnectionAdapter,
	headerParser ip.HeaderParser,
	serviceFrame *Frame,
) *Adapter {
	return &Adapter{
		adapter:      adapter,
		headerParser: headerParser,
		serviceFrame: serviceFrame,
	}
}

func (a *Adapter) Write(data []byte) (int, error) {
	version, versionErr := a.headerParser.Version(data)
	if versionErr != nil {
		// not an ip packet, passthrough
		return a.adapter.Write(data)
	}

	var payload []byte
	var proto uint8
	switch version {
	case 4:
		// Minimal safe parsing for v4
		if len(data) < ipv4.HeaderLen {
			return a.adapter.Write(data)
		}
		ihl := int(data[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen || len(data) < ihl {
			return a.adapter.Write(data)
		}
		proto = data[9]
		payload = data[ihl:]
	case 6:
		if len(data) < ipv6.HeaderLen {
			return a.adapter.Write(data)
		}
		proto = data[6]
		payload = data[ipv6.HeaderLen:]
	default:
		return a.adapter.Write(data)
	}

	// Quick SF signature check to avoid allocations.
	if len(payload) < domain.HeaderSize ||
		payload[0] != domain.MagicSF[0] || payload[1] != domain.MagicSF[1] {
		return a.adapter.Write(data)
	}

	// Validate SF
	if err := a.serviceFrame.UnmarshalBinary(payload); err != nil {
		return a.adapter.Write(data)
	}

	// We only handle MTUProbe here; others pass-through.
	if a.serviceFrame.kind != domain.KindMTUProbe {
		return a.adapter.Write(data)
	}

	// Extract src/dst from the IP header
	dst, err := a.headerParser.DestinationAddress(data)
	if err != nil {
		return 0, err
	}
	src, err := a.headerParser.SourceAddress(data)
	if err != nil {
		return 0, err
	}

	// Swap dst with src. This client's packet will be going backward, i.e. from server to client.
	reply, err := a.mtuProbeHandler.Respond(dst, src, version, proto, payload)
	if err != nil {
		return 0, err
	}

	// Write the reply packet to TUN (so egress pipeline will encrypt & send to client).
	return a.adapter.Write(reply)
}

func (a *Adapter) Read(buffer []byte) (int, error) {
	return a.adapter.Read(buffer)
}

func (a *Adapter) Close() error {
	return a.adapter.Close()
}
