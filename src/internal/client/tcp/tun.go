package tcp

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/ip"
)

type rekeyInitiator interface {
	MaybeBuildRekeyInit(now time.Time, dst []byte) (payload []byte, ok bool, err error)
}

type tunHandler struct {
	ctx              context.Context
	reader           io.Reader // abstraction over TUN device
	egress           sender
	rekeyInit        rekeyInitiator
	allowedSources   map[netip.Addr]struct{}
	controlPacketBuf [settings.DefaultEthernetMTU + chacha20.TCPOverhead]byte
}

func newTunHandler(ctx context.Context,
	reader io.Reader,
	egress sender,
	rekeyInit rekeyInitiator,
	allowedSources map[netip.Addr]struct{},
) *tunHandler {
	return &tunHandler{
		ctx:            ctx,
		reader:         reader,
		egress:         egress,
		rekeyInit:      rekeyInit,
		allowedSources: allowedSources,
	}
}

func (t *tunHandler) HandleTun() error {
	// Buffer layout: [2B epoch reserved][plaintext up to MTU][16B AEAD tag capacity]
	var buffer [settings.DefaultEthernetMTU + chacha20.TCPOverhead]byte
	payload := buffer[chacha20.TCPEpochPrefixSize : settings.DefaultEthernetMTU+chacha20.TCPEpochPrefixSize]

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
			if err := t.egress.Send(buffer[:chacha20.TCPEpochPrefixSize+n]); err != nil {
				slog.Error("write to TCP failed", "err", err)
				return err
			}

			if t.rekeyInit != nil {
				now := time.Now().UTC()
				dst := t.controlPacketBuf[chacha20.TCPEpochPrefixSize : chacha20.TCPEpochPrefixSize+settings.DefaultEthernetMTU]
				servicePayload, ok, err := t.rekeyInit.MaybeBuildRekeyInit(now, dst)
				if err != nil {
					slog.Warn("failed to prepare rekey init", "err", err)
					continue
				}
				if ok {
					spWithPrefix := t.controlPacketBuf[:chacha20.TCPEpochPrefixSize+len(servicePayload)]
					if err := t.egress.Send(spWithPrefix); err != nil {
						slog.Warn("failed to send rekey init", "err", err)
					}
				}
			}
		}
	}
}
