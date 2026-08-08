package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"

	"golang.org/x/crypto/chacha20poly1305"

	"tungo/application/configuration/settings"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
)

// Server is the complete TCP datapath. RunTransport moves authenticated packets
// from client transports to TUN; RunTun routes TUN packets back to client transports.
type Server struct {
	ctx       context.Context
	tun       io.ReadWriter
	listener  net.Listener
	peers     *session.Repository
	registrar *tcp_registration.Registrar
	deriver   primitives.DefaultKeyDeriver
}

func NewServer(
	ctx context.Context,
	tun io.ReadWriter,
	listener net.Listener,
	peers *session.Repository,
	registrar *tcp_registration.Registrar,
) *Server {
	return &Server{
		ctx: ctx, tun: tun, listener: listener,
		peers: peers, registrar: registrar,
	}
}

func (s *Server) RunTransport() error {
	defer func() { _ = s.listener.Close() }()

	go func() {
		<-s.ctx.Done()
		_ = s.listener.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if s.ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Warn("failed to accept connection", "err", err)
			continue
		}
		go s.serveClient(conn)
	}
}

func (s *Server) serveClient(conn net.Conn) {
	peer, transport, err := s.registrar.RegisterClient(conn)
	if err != nil {
		slog.Warn("failed to register client", "err", err)
		return
	}
	defer func() {
		s.peers.Delete(peer)
		_ = transport.Close()
		slog.Info("client disconnected", "peer", peer.ExternalAddrPort())
	}()

	var frame [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte

	for {
		if s.ctx.Err() != nil {
			return
		}
		n, err := transport.Read(frame[:])
		if err != nil {
			if err != io.EOF {
				slog.Warn("failed to read from client", "err", err)
			}
			return
		}
		err = s.handleFrame(peer, frame[:n])
		if errors.Is(err, session.ErrPeerClosed) {
			return
		}
		if err != nil {
			slog.Warn("failed to handle client frame", "err", err)
			return
		}
	}
}

func (s *Server) handleFrame(peer *session.Peer, frame []byte) error {
	if len(frame) < tcp.EpochPrefixSize+chacha20poly1305.Overhead || len(frame) > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
		return nil
	}
	plaintext, err := peer.Decrypt(frame)
	if err != nil {
		return err
	}
	epoch := binary.BigEndian.Uint16(frame[:tcp.EpochPrefixSize])
	if rekey := peer.RekeyController(); rekey != nil {
		rekey.ObservePeerEpoch(epoch)
	}
	if handled, err := s.handleService(peer, epoch, plaintext); handled {
		return err
	}
	source, ok := ip.ExtractSourceIP(plaintext)
	if !ok || !peer.IsSourceAllowed(source) {
		return nil
	}
	if _, err := s.tun.Write(plaintext); err != nil {
		return err
	}
	return nil
}

func (s *Server) RunTun() error {
	var frame [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte
	plaintext := frame[tcp.EpochPrefixSize : tcp.EpochPrefixSize+settings.DefaultEthernetMTU]

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
		if err := peer.Send(frame[:tcp.EpochPrefixSize+n]); err != nil {
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
		s.sendPong(peer)
	}
	return true, nil
}

func (s *Server) handleRekey(peer *session.Peer, carrierEpoch uint16, plaintext []byte) error {
	controller := peer.RekeyController()
	if controller == nil {
		return nil
	}
	serverPub, epoch, ok, err := controller.HandleRekeyInit(carrierEpoch, &s.deriver, plaintext)
	if err != nil {
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			return s.sendService(peer, service_packet.EpochExhausted, nil)
		}
		return nil
	}
	if !ok {
		return nil
	}
	if err := s.sendService(peer, service_packet.RekeyAck, serverPub[:]); err != nil {
		return err
	}
	controller.ActivateSendEpoch(epoch)
	return nil
}

func (s *Server) sendPong(peer *session.Peer) {
	if err := s.sendService(peer, service_packet.Pong, nil); err != nil {
		slog.Warn("failed to send pong", "err", err)
	}
}

func (s *Server) sendService(peer *session.Peer, kind service_packet.HeaderType, body []byte) error {
	var frame [tcp.EpochPrefixSize + service_packet.RekeyPacketLen + settings.TCPChacha20Overhead]byte
	payloadLen := 3 + len(body)
	payload := frame[tcp.EpochPrefixSize : tcp.EpochPrefixSize+payloadLen]
	copy(payload[3:], body)
	if _, err := service_packet.EncodeV1Header(kind, payload); err != nil {
		return err
	}
	return peer.Send(frame[:tcp.EpochPrefixSize+payloadLen])
}
