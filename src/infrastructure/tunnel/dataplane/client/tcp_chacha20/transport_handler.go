package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// ErrEpochExhausted is returned when server signals epoch exhaustion.
// Client should reconnect with a fresh handshake.
var ErrEpochExhausted = errors.New("epoch exhausted; reconnect required")

type transportRekeyController interface {
	ObservePeerEpoch(epoch uint16)
}

type rekeyAckHandler interface {
	HandleRekeyAck(carrierEpoch uint16, plaindata []byte) (ok bool, err error)
}

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     transportRekeyController
	rekeyAck            rekeyAckHandler
	egress              connection.Egress
	lastRecvNano        atomic.Int64
	pingBuf             []byte
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController transportRekeyController,
	rekeyAck rekeyAckHandler,
	egress connection.Egress,
) transport.Handler {
	t := &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		rekeyAck:            rekeyAck,
		egress:              egress,
		pingBuf:             make([]byte, tcp.EpochPrefixSize+3, tcp.EpochPrefixSize+3+settings.TCPChacha20Overhead),
	}
	t.lastRecvNano.Store(time.Now().UnixNano())
	return t
}

func (t *TransportHandler) HandleTransport() error {
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
			if t.rekeyController != nil {
				t.rekeyController.ObservePeerEpoch(carrierEpoch)
			}

			t.lastRecvNano.Store(time.Now().UnixNano())

			if spType, spOk := service_packet.TryParseHeader(payload); spOk {
				switch spType {
				case service_packet.EpochExhausted:
					slog.Warn("received EpochExhausted from server, initiating reconnect")
					return ErrEpochExhausted
				case service_packet.RekeyAck:
					if err := t.handleRekeyAck(carrierEpoch, payload); err != nil {
						return err
					}
					continue
				case service_packet.Pong:
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

func (t *TransportHandler) keepaliveLoop() {
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

func (t *TransportHandler) sendPing() {
	payload := t.pingBuf[tcp.EpochPrefixSize:]
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, payload); err != nil {
		slog.Warn("keepalive failed to encode ping", "err", err)
		return
	}
	if err := t.egress.Send(t.pingBuf[:]); err != nil {
		slog.Warn("keepalive failed to send ping", "err", err)
	}
}

func (t *TransportHandler) handleRekeyAck(carrierEpoch uint16, payload []byte) error {
	if t.rekeyAck == nil {
		return nil
	}
	_, err := t.rekeyAck.HandleRekeyAck(carrierEpoch, payload)
	if err != nil {
		slog.Error("rekey ack install/apply failed", "err", err)
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			return ErrEpochExhausted
		}
	}
	return nil
}
