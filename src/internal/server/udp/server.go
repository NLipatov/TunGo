package udp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	udpcrypto "tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/ip"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	"tungo/internal/server/session"
)

// Server moves packets between a TUN device and UDP client transports.
type Server struct {
	ctx       context.Context
	tun       io.ReadWriter
	conn      *net.UDPConn
	peers     *session.Repository
	registrar *registrar
	deriver   keys.DefaultKeyDeriver
}

// New configures conn for receiving destination addresses. On error, the caller
// retains ownership of conn and must close it.
func New(
	ctx context.Context,
	tun io.ReadWriter,
	conn *net.UDPConn,
	peers *session.Repository,
	newHandshake func() *noise.IKHandshake,
	ipv4Subnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) (*Server, error) {
	if err := setControlMessage(conn); err != nil {
		return nil, fmt.Errorf("enable UDP destination control messages: %w", err)
	}
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
	}, nil
}

func setControlMessage(conn *net.UDPConn) error {
	// ListenUDP("udp", 0.0.0.0) may create a dual-stack IPv6 socket.
	// IPv6 packet info also reports destinations of mapped IPv4 datagrams.
	if conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Is4() {
		return ipv4.NewPacketConn(conn).SetControlMessage(ipv4.FlagDst, true)
	}
	return ipv6.NewPacketConn(conn).SetControlMessage(ipv6.FlagDst, true)
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
		n, oobn, _, addr, err := s.conn.ReadMsgUDPAddrPort(frame[:], oob[:])
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
		if err := s.handleDatagram(addr, frame[:n], oob[:oobn]); err != nil {
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

func (s *Server) handleDatagram(clientAddrPort netip.AddrPort, frame, oob []byte) error {
	routeID, ok := udpcrypto.ReadRouteID(frame)
	if !ok {
		return nil
	}
	peer, err := s.peers.GetByRouteID(routeID)
	if err != nil {
		if s.registrar != nil {
			serverAddr, err := datagramDestination(oob)
			if err != nil {
				return err
			}
			s.registrar.enqueuePacket(registrationKey{clientAddrPort: clientAddrPort, serverAddr: serverAddr}, frame)
		}
		return nil
	}
	return s.handleEstablished(clientAddrPort, peer, frame)
}

// Copy the destination out of the receive buffer before it is reused. A missing
// destination must not silently fall back to the kernel's default reply source.
func datagramDestination(oob []byte) (netip.Addr, error) {
	var cm4 ipv4.ControlMessage
	if err := cm4.Parse(oob); err != nil {
		return netip.Addr{}, fmt.Errorf("parse UDP IPv4 control message: %w", err)
	}
	if addr, ok := netip.AddrFromSlice(cm4.Dst); ok {
		return addr.Unmap(), nil
	}
	var cm6 ipv6.ControlMessage
	if err := cm6.Parse(oob); err != nil {
		return netip.Addr{}, fmt.Errorf("parse UDP IPv6 control message: %w", err)
	}
	if addr, ok := netip.AddrFromSlice(cm6.Dst); ok {
		return addr.Unmap(), nil
	}
	return netip.Addr{}, errors.New("UDP destination control message missing")
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
	plaintext := frame[udpcrypto.PayloadOffset : udpcrypto.PayloadOffset+settings.DefaultEthernetMTU]

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
		if err := peer.Send(frame[:udpcrypto.PayloadOffset+n]); err != nil {
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
	payload := frame[udpcrypto.PayloadOffset : udpcrypto.PayloadOffset+payloadLen]
	copy(payload[3:], body)
	if err := servicepacket.Encode(kind, payload); err != nil {
		return err
	}
	return peer.Send(frame[:udpcrypto.PayloadOffset+payloadLen])
}

func (*Server) sendPlaintext(peer *session.Peer, payload []byte) error {
	if len(payload) > settings.DefaultEthernetMTU {
		return io.ErrShortBuffer
	}
	var frame [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	copy(frame[udpcrypto.PayloadOffset:], payload)
	return peer.Send(frame[:udpcrypto.PayloadOffset+len(payload)])
}
