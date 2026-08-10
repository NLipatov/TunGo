package tcp

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/tcp"
	"tungo/internal/protocol/servicepacket"

	"golang.org/x/crypto/chacha20poly1305"
)

// errEpochExhausted is returned when server signals epoch exhaustion.
// Client should reconnect with a fresh handshake.
var errEpochExhausted = errors.New("epoch exhausted; reconnect required")

type transportRekey interface {
	ObservePeerEpoch(epoch uint16)
	HandleRekeyAck(carrierEpoch uint16, plaindata []byte) (ok bool, err error)
}

type transportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService crypto
	rekey               transportRekey
	egress              sender
	lastRecvNano        atomic.Int64
	pingBuf             []byte
}

func newTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService crypto,
	rekey transportRekey,
	egress sender,
) *transportHandler {
	t := &transportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekey:               rekey,
		egress:              egress,
		pingBuf:             make([]byte, tcp.EpochPrefixSize+3, tcp.EpochPrefixSize+3+settings.TCPChacha20Overhead),
	}
	t.lastRecvNano.Store(time.Now().UnixNano())
	return t
}

func (t *transportHandler) HandleTransport() error {
	go t.keepaliveLoop()

	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer[:])
			if readErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				slog.Warn("read from TCP failed", "err", readErr)
				return readErr
			}

			if n < chacha20poly1305.Overhead || n > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
				continue
			}

			payload, payloadErr := t.cryptographyService.Decrypt(buffer[:n])
			if payloadErr != nil {
				slog.Warn("failed to decrypt data", "err", payloadErr)
				return payloadErr
			}
			carrierEpoch := binary.BigEndian.Uint16(buffer[:tcp.EpochPrefixSize])
			if t.rekey != nil {
				t.rekey.ObservePeerEpoch(carrierEpoch)
			}

			t.lastRecvNano.Store(time.Now().UnixNano())

			if spType, spOk := servicepacket.Parse(payload); spOk {
				switch spType {
				case servicepacket.EpochExhausted:
					slog.Warn("received EpochExhausted from server, initiating reconnect")
					return errEpochExhausted
				case servicepacket.RekeyAck, servicepacket.RekeyAckV2:
					if err := t.handleRekeyAck(carrierEpoch, payload); err != nil {
						return err
					}
					continue
				case servicepacket.Pong:
					continue
				}
			}
			if _, writeErr := t.writer.Write(payload); writeErr != nil {
				slog.Error("failed to write to TUN", "err", writeErr)
				return writeErr
			}
		}
	}
}

func (t *transportHandler) keepaliveLoop() {
	ticker := time.NewTicker(settings.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			lastRecv := time.Unix(0, t.lastRecvNano.Load())
			if t.egress != nil && time.Since(lastRecv) > settings.PingInterval {
				t.sendPing()
			}
		}
	}
}

func (t *transportHandler) sendPing() {
	payload := t.pingBuf[tcp.EpochPrefixSize:]
	if err := servicepacket.Encode(servicepacket.Ping, payload); err != nil {
		slog.Warn("keepalive failed to encode ping", "err", err)
		return
	}
	if err := t.egress.Send(t.pingBuf[:]); err != nil {
		slog.Warn("keepalive failed to send ping", "err", err)
	}
}

func (t *transportHandler) handleRekeyAck(carrierEpoch uint16, payload []byte) error {
	if t.rekey == nil {
		return nil
	}
	_, err := t.rekey.HandleRekeyAck(carrierEpoch, payload)
	if err != nil {
		slog.Error("rekey ack install/apply failed", "err", err)
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			return errEpochExhausted
		}
	}
	return nil
}
