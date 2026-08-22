package trafficstats

import "io"

type meteredTun struct {
	io.ReadWriter
	collector *Collector
}

// WrapTun counts plaintext IP bytes at the TUN boundary.
// Reading from TUN is outbound traffic; writing to TUN is inbound traffic.
func WrapTun(tun io.ReadWriter) io.ReadWriter {
	collector := Global()
	if collector == nil {
		return tun
	}
	return &meteredTun{ReadWriter: tun, collector: collector}
}

func (t *meteredTun) Read(p []byte) (int, error) {
	n, err := t.ReadWriter.Read(p)
	t.collector.AddTX(n)
	return n, err
}

func (t *meteredTun) Write(p []byte) (int, error) {
	n, err := t.ReadWriter.Write(p)
	t.collector.AddRX(n)
	return n, err
}
