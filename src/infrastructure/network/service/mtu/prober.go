package mtu

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"tungo/application"
	domain "tungo/domain/network/serviceframe"
)

type Prober struct {
	ctx                               context.Context
	adapter                           application.ConnectionAdapter
	bestMTU                           int
	maximum, minimum                  int
	probeReadBuffer, probeWriteBuffer [1500]byte
}

func NewProber(ctx context.Context, adapter application.ConnectionAdapter, minimum, maximum int) *Prober {
	if minimum > maximum {
		minimum, maximum = maximum, minimum
	}
	return &Prober{
		ctx:     ctx,
		adapter: adapter,
		maximum: maximum,
		minimum: minimum,
	}
}

func (p *Prober) MTU() (int, error) {
	p.bestMTU = -1
	p.discoverMTU(p.minimum, p.maximum)
	if p.bestMTU == -1 {
		return 0, fmt.Errorf("failed to discover best MTU")
	}
	return p.bestMTU, nil
}

func (p *Prober) discoverMTU(minimum, maximum int) {
	if minimum > maximum {
		return
	}
	current := minimum + (maximum-minimum)/2
	if err := p.tryMtu(current); err != nil {
		p.discoverMTU(minimum, current-1)
	} else {
		p.bestMTU = current
		p.discoverMTU(current+1, maximum)
	}
}

func (p *Prober) tryMtu(mtu int) error {
	// payload length = mtu - HeaderSize
	if mtu < domain.HeaderSize {
		return fmt.Errorf("mtu too small")
	}
	payloadLen := mtu - domain.HeaderSize
	if payloadLen > int(domain.MaxBody) {
		return fmt.Errorf("mtu too large for serviceframe payload")
	}

	ctx, cancel := context.WithDeadline(p.ctx, time.Now().Add(500*time.Millisecond))
	defer cancel()

	type result struct {
		err error
		mtu int
	}
	resChan := make(chan result, 1)

	// Reader goroutine: wait for ONE valid MTUAck, then return.
	go func() {
		var frame domain.Frame
		for {
			select {
			case <-ctx.Done():
				resChan <- result{err: ctx.Err(), mtu: 0}
				return
			default:
				n, readErr := p.adapter.Read(p.probeReadBuffer[:])
				if readErr != nil || n < domain.HeaderSize {
					continue
				}
				// Quick magic check.
				if p.probeReadBuffer[0] != domain.MagicSF[0] || p.probeReadBuffer[1] != domain.MagicSF[1] {
					continue
				}
				if err := frame.UnmarshalBinary(p.probeReadBuffer[:n]); err != nil {
					continue
				}
				if frame.Kind() != domain.KindMTUAck {
					continue
				}
				body := frame.Body()
				if len(body) < 2 {
					continue
				}
				acked := int(binary.BigEndian.Uint16(body[:2])) + domain.HeaderSize
				resChan <- result{err: nil, mtu: acked}
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r := <-resChan:
			// accept only exact echo of current mtu
			if r.err == nil && r.mtu == mtu {
				return nil
			}
			// otherwise keep trying until deadline
		default:
			// Build and send a probe with payload length = mtu - HeaderSize
			frame, err := domain.NewFrame(
				domain.V1,
				domain.KindMTUProbe,
				domain.FlagNone,
				p.probeWriteBuffer[:payloadLen],
			)
			if err != nil {
				return err
			}
			wire, err := frame.MarshalBinary()
			if err != nil {
				return err
			}
			if _, err = p.adapter.Write(wire); err != nil {
				return err
			}
			// light pacing to avoid flooding
			time.Sleep(25 * time.Millisecond)
		}
	}
}
