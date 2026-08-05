package tcp_chacha20

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
)

// epochPrefixSize is the number of bytes reserved at the start of the buffer
// for the 2-byte epoch tag prepended to every TCP ciphertext frame.
const epochPrefixSize = 2

type rekeyInitiator interface {
	MaybeBuildRekeyInit(now time.Time, dst []byte) (payload []byte, ok bool, err error)
}

type TunHandler struct {
	ctx              context.Context
	reader           io.Reader // abstraction over TUN device
	egress           connection.Egress
	rekeyInit        rekeyInitiator
	allowedSources   map[netip.Addr]struct{}
	controlPacketBuf [epochPrefixSize + service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	egress connection.Egress,
	rekeyInit rekeyInitiator,
	allowedSources map[netip.Addr]struct{},
) tun.Handler {
	return &TunHandler{
		ctx:            ctx,
		reader:         reader,
		egress:         egress,
		rekeyInit:      rekeyInit,
		allowedSources: allowedSources,
	}
}

func (t *TunHandler) HandleTun() error {
	// Buffer layout: [2B epoch reserved][plaintext up to MTU][16B AEAD tag capacity]
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	payload := buffer[epochPrefixSize : settings.DefaultEthernetMTU+epochPrefixSize]
	rec := trafficstats.NewRecorder()
	defer rec.Flush()

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(payload)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				slog.Error("failed to read from TUN", "err", err)
				return err
			}

			if len(t.allowedSources) > 0 && !ip.IsAllowedSource(payload[:n], t.allowedSources) {
				continue
			}

			// Pass buffer including the 2-byte epoch prefix reservation.
			if err := t.egress.SendDataIP(buffer[:epochPrefixSize+n]); err != nil {
				slog.Error("write to TCP failed", "err", err)
				return err
			}
			rec.RecordTX(uint64(n))

			if t.rekeyInit != nil {
				now := time.Now().UTC()
				dst := t.controlPacketBuf[epochPrefixSize : epochPrefixSize+service_packet.RekeyPacketLen]
				servicePayload, ok, err := t.rekeyInit.MaybeBuildRekeyInit(now, dst)
				if err != nil {
					slog.Warn("failed to prepare rekey init", "err", err)
					continue
				}
				if ok {
					spWithPrefix := t.controlPacketBuf[:epochPrefixSize+len(servicePayload)]
					if err := t.egress.SendControl(spWithPrefix); err != nil {
						slog.Warn("failed to send rekey init", "err", err)
					}
				}
			}
		}
	}
}
