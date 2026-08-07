package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	"golang.org/x/crypto/chacha20poly1305"

	"tungo/application/listeners"
	"tungo/infrastructure/cryptography/chacha20"
	udpcrypto "tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

const udpPayloadOffset = udpcrypto.PayloadOffset

// Server is the complete UDP datapath. RunTransport moves authenticated
// datagrams to TUN; RunTun routes TUN packets to established peers.
type Server struct {
	ctx       context.Context
	settings  settings.Settings
	tun       io.ReadWriter
	conn      listeners.UdpListener
	peers     *session.DefaultRepository
	registrar *udp_registration.Registrar
	deriver   primitives.DefaultKeyDeriver
}

func NewServer(
	ctx context.Context,
	settings settings.Settings,
	tun io.ReadWriter,
	conn listeners.UdpListener,
	peers *session.DefaultRepository,
	registrar *udp_registration.Registrar,
) *Server {
	return &Server{
		ctx: ctx, settings: settings, tun: tun, conn: conn,
		peers: peers, registrar: registrar,
	}
}

func (s *Server) RunTransport() error {
	defer func() { _ = s.conn.Close() }()
	slog.Info("server listening", "protocol", "UDP", "port", s.settings.Port)

	go session.RunIdleReaperLoop(
		s.ctx, s.peers.ReapIdle, settings.ServerIdleTimeout, settings.IdleReaperInterval,
	)
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

func (s *Server) closeRegistrations() {
	if s.registrar != nil {
		s.registrar.CloseAll()
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
			s.registrar.EnqueuePacket(addr, frame)
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
	if rekey := peer.RekeyController(); rekey != nil {
		rekey.ObservePeerEpoch(epoch)
		rekey.ActivateSendEpoch(epoch)
	}
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

func (s *Server) RunTun() error {
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
	kind, ok := service_packet.TryParseHeader(plaintext)
	if !ok {
		return false, nil
	}
	switch kind {
	case service_packet.RekeyInit:
		return true, s.handleRekey(peer, carrierEpoch, plaintext)
	case service_packet.Ping:
		return true, s.sendService(peer, service_packet.Pong, nil)
	}
	return true, nil
}

func (s *Server) handleRekey(peer *session.Peer, carrierEpoch uint16, plaintext []byte) error {
	controller := peer.RekeyController()
	if controller == nil {
		return nil
	}
	serverPub, _, ok, err := controller.HandleRekeyInit(carrierEpoch, &s.deriver, plaintext)
	if err != nil {
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			_ = s.sendService(peer, service_packet.EpochExhausted, nil)
		}
		return nil
	}
	if !ok {
		return nil
	}
	return s.sendService(peer, service_packet.RekeyAck, serverPub[:])
}

func (s *Server) sendService(peer *session.Peer, kind service_packet.HeaderType, body []byte) error {
	var frame [udpPayloadOffset + service_packet.RekeyPacketLen + chacha20poly1305.Overhead]byte
	payloadLen := 3 + len(body)
	payload := frame[udpPayloadOffset : udpPayloadOffset+payloadLen]
	copy(payload[3:], body)
	if _, err := service_packet.EncodeV1Header(kind, payload); err != nil {
		return err
	}
	return peer.Send(frame[:udpPayloadOffset+payloadLen])
}
