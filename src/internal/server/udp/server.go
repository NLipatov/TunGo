package udp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	udpcrypto "tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/ip"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	"tungo/internal/server/session"
	transport "tungo/internal/transport/udp"
)

const udpPayloadOffset = udpcrypto.PayloadOffset

// Server moves packets between a TUN device and UDP client transports.
type Server struct {
	ctx       context.Context
	tun       io.ReadWriter
	conn      transport.UdpListener
	peers     *session.Repository
	registrar *registrar
	deriver   keys.DefaultKeyDeriver
}

func New(
	ctx context.Context,
	tun io.ReadWriter,
	conn transport.UdpListener,
	peers *session.Repository,
	newHandshake func() *noise.IKHandshake,
	ipv4Subnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) *Server {
	return &Server{
		ctx: ctx, tun: tun, conn: conn,
		peers: peers,
		registrar: newRegistrar(
			ctx,
			conn,
			peers,
			func() handshake { return newHandshake() },
			func(material chacha20.KeyMaterial, isServer bool) (crypto, epochController, error) {
				return udpcrypto.NewFromHandshake(material, isServer)
			},
			ipv4Subnet,
			ipv6Subnet,
		),
	}
}

// Run moves packets in both directions until the context is cancelled or one
// direction fails.
func (s *Server) Run() error {
	errCh := make(chan error, 2)
	go func() { errCh <- s.runTun() }()
	go func() { errCh <- s.runTransport() }()

	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Server) runTransport() error {
	defer func() { _ = s.conn.Close() }()

	go s.reapIdlePeers()
	_ = s.conn.SetReadBuffer(4 * 1024 * 1024)
	_ = s.conn.SetWriteBuffer(4 * 1024 * 1024)
	go func() {
		<-s.ctx.Done()
		_ = s.conn.Close()
	}()

	var frame [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	var oob [1024]byte
	for {
		n, _, _, addr, err := s.conn.ReadMsgUDPAddrPort(frame[:], oob[:])
		if err != nil {
			if s.ctx.Err() != nil {
				s.closeRegistrations()
				return nil
			}
			slog.Warn("failed to read from UDP", "err", err)
			continue
		}
		if n == 0 {
			continue
		}
		if err := s.handleDatagram(addr, frame[:n]); err != nil {
			slog.Warn("failed to handle UDP packet", "err", err)
		}
	}
}

func (s *Server) reapIdlePeers() {
	ticker := time.NewTicker(settings.IdleReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if count := s.peers.ReapIdle(settings.ServerIdleTimeout); count > 0 {
				slog.Info("reaped idle sessions", "count", count)
			}
		}
	}
}

func (s *Server) closeRegistrations() {
	if s.registrar != nil {
		s.registrar.closeAll()
	}
}

func (s *Server) handleDatagram(addr netip.AddrPort, frame []byte) error {
	routeID, ok := udpcrypto.ReadRouteID(frame)
	if !ok {
		return nil
	}
	peer, err := s.peers.GetByRouteID(routeID)
	if err != nil {
		if s.registrar != nil {
			s.registrar.enqueuePacket(addr, frame)
		}
		return nil
	}
	return s.handleEstablished(addr, peer, frame)
}

func (s *Server) handleEstablished(addr netip.AddrPort, peer *session.Peer, frame []byte) error {
	plaintext, err := peer.Decrypt(frame)
	if err != nil {
		return nil
	}
	if peer.ExternalAddrPort() != addr {
		s.peers.UpdateExternalAddr(peer, addr)
	}
	return s.handleDecrypted(peer, frame, plaintext)
}

func (s *Server) handleDecrypted(
	peer *session.Peer,
	frame, plaintext []byte,
) error {
	if peer.IsClosed() {
		return nil
	}
	peer.TouchActivity()

	if len(frame) < udpcrypto.EpochOffset+2 {
		return nil
	}
	epoch := binary.BigEndian.Uint16(frame[udpcrypto.EpochOffset : udpcrypto.EpochOffset+2])
	peer.ObservePeerEpoch(epoch)
	peer.ActivateSendEpoch(epoch)
	if handled, err := s.handleService(peer, epoch, plaintext); handled {
		return err
	}

	source, ok := ip.ExtractSourceIP(plaintext)
	if !ok || !peer.IsSourceAllowed(source) {
		return nil
	}
	if _, err := s.tun.Write(plaintext); err != nil {
		return fmt.Errorf("write to TUN: %w", err)
	}
	return nil
}

func (s *Server) runTun() error {
	var frame [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	plaintext := frame[udpPayloadOffset : udpPayloadOffset+settings.DefaultEthernetMTU]

	for {
		if s.ctx.Err() != nil {
			return nil
		}
		n, err := s.tun.Read(plaintext)
		if err != nil {
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				continue
			}
			return err
		}
		if n == 0 {
			continue
		}
		destination, ok := ip.ExtractDestIP(plaintext[:n])
		if !ok {
			continue
		}
		peer, err := s.peers.FindByDestinationIP(destination)
		if err != nil {
			continue
		}
		if err := peer.Send(frame[:udpPayloadOffset+n]); err != nil {
			slog.Warn("failed to send packet to peer", "peer", peer.ExternalAddrPort(), "err", err)
			s.peers.Delete(peer)
			continue
		}
	}
}

func (s *Server) handleService(peer *session.Peer, carrierEpoch uint16, plaintext []byte) (bool, error) {
	if kind, ok := servicepacket.Parse(plaintext); ok {
		switch kind {
		case servicepacket.RekeyInit, servicepacket.RekeyInitV2:
			return true, s.handleRekey(peer, carrierEpoch, plaintext)
		case servicepacket.Ping:
			return true, s.sendService(peer, servicepacket.Pong, nil)
		}
		return true, nil
	}
	return false, nil
}

func (s *Server) handleRekey(peer *session.Peer, carrierEpoch uint16, plaintext []byte) error {
	response, _, rekeyed, err := peer.HandleRekey(carrierEpoch, &s.deriver, plaintext)
	if err != nil {
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			_ = s.sendService(peer, servicepacket.EpochExhausted, nil)
		}
		return nil
	}
	if rekeyed {
		return s.sendPlaintext(peer, response)
	}
	return nil
}

func (s *Server) sendService(peer *session.Peer, kind servicepacket.HeaderType, body []byte) error {
	var frame [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	payloadLen := 3 + len(body)
	if payloadLen > settings.DefaultEthernetMTU {
		return io.ErrShortBuffer
	}
	payload := frame[udpPayloadOffset : udpPayloadOffset+payloadLen]
	copy(payload[3:], body)
	if err := servicepacket.Encode(kind, payload); err != nil {
		return err
	}
	return peer.Send(frame[:udpPayloadOffset+payloadLen])
}

func (*Server) sendPlaintext(peer *session.Peer, payload []byte) error {
	if len(payload) > settings.DefaultEthernetMTU {
		return io.ErrShortBuffer
	}
	var frame [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	copy(frame[udpPayloadOffset:], payload)
	return peer.Send(frame[:udpPayloadOffset+len(payload)])
}
