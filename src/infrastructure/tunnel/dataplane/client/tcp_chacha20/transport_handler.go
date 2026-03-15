package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

// ErrEpochExhausted is returned when server signals epoch exhaustion.
// Client should reconnect with a fresh handshake.
var ErrEpochExhausted = errors.New("epoch exhausted; reconnect required")

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     *rekey.StateMachine
	handshakeCrypto     primitives.KeyDeriver
	egress              connection.Egress
	lastRecvNano        atomic.Int64
	pingBuf             []byte
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.StateMachine,
	egress connection.Egress,
) transport.Handler {
	t := &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		handshakeCrypto:     &primitives.DefaultKeyDeriver{},
		egress:              egress,
		pingBuf:             make([]byte, epochPrefixSize+3, epochPrefixSize+3+settings.TCPChacha20Overhead),
	}
	t.lastRecvNano.Store(time.Now().UnixNano())
	return t
}

func (t *TransportHandler) HandleTransport() error {
	go t.keepaliveLoop()

	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	rec := trafficstats.NewRecorder()
	defer rec.Flush()

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

			t.lastRecvNano.Store(time.Now().UnixNano())

			if spType, spOk := service_packet.TryParseHeader(payload); spOk {
				switch spType {
				case service_packet.EpochExhausted:
					slog.Warn("received EpochExhausted from server, initiating reconnect")
					return ErrEpochExhausted
				case service_packet.RekeyAck:
					if err := t.handleRekeyAck(payload); err != nil {
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
			rec.RecordRX(uint64(len(payload)))
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
	payload := t.pingBuf[epochPrefixSize:]
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, payload); err != nil {
		slog.Warn("keepalive failed to encode ping", "err", err)
		return
	}
	if err := t.egress.SendControl(t.pingBuf[:]); err != nil {
		slog.Warn("keepalive failed to send ping", "err", err)
	}
}

func (t *TransportHandler) handleRekeyAck(payload []byte) error {
	if t.rekeyController == nil {
		return nil
	}
	if t.rekeyController.LastRekeyEpoch >= 65000 {
		slog.Warn("rekey ack exhausted epoch, requesting session reset")
		return ErrEpochExhausted
	}
	_, err := controlplane.ClientHandleRekeyAck(t.handshakeCrypto, t.rekeyController, payload)
	if err != nil {
		slog.Error("rekey ack install/apply failed", "err", err)
	}
	return nil
}
