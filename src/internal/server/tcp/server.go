package tcp

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"

	"golang.org/x/crypto/chacha20poly1305"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	tcpcrypto "tungo/internal/protocol/chacha20/tcp"
	"tungo/internal/protocol/ip"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	"tungo/internal/server/session"
)

// Server moves packets between a TUN device and TCP-style client transports.
type Server struct {
	ctx       context.Context
	tun       io.ReadWriter
	listener  net.Listener
	peers     *session.Repository
	registrar *registrar
	deriver   keys.DefaultKeyDeriver
}

func New(
	ctx context.Context,
	tun io.ReadWriter,
	listener net.Listener,
	peers *session.Repository,
	newHandshake func() *noise.IKHandshake,
	ipv4Subnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) *Server {
	return &Server{
		ctx: ctx, tun: tun, listener: listener,
		peers: peers,
		registrar: newRegistrar(
			func() handshake { return newHandshake() },
			func(material chacha20.KeyMaterial, isServer bool) (crypto, epochController, error) {
				return tcpcrypto.NewFromHandshake(material, isServer)
			},
			peers,
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
	peer, sender, transport, err := s.registrar.registerClient(conn)
	if err != nil {
		slog.Warn("failed to register client", "err", err)
		return
	}
	defer func() {
		s.peers.Delete(peer)
		_ = transport.Close()
		slog.Info("client disconnected", "peer", peer.ExternalAddrPort())
	}()

	var frame [settings.DefaultEthernetMTU + chacha20.TCPOverhead]byte

	for {
		if s.ctx.Err() != nil {
			return
		}
		n, err := transport.Read(frame[:])
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Warn("failed to read from client", "err", err)
			}
			return
		}
		err = s.handleFrame(peer, sender, frame[:n])
		if errors.Is(err, session.ErrPeerClosed) {
			return
		}
		if err != nil {
			slog.Warn("failed to handle client frame", "err", err)
			return
		}
	}
}

func (s *Server) handleFrame(peer *session.Peer, sender *peerSender, frame []byte) error {
	if len(frame) < chacha20.TCPEpochPrefixSize+chacha20poly1305.Overhead || len(frame) > settings.DefaultEthernetMTU+chacha20.TCPOverhead {
		return nil
	}
	plaintext, err := peer.Decrypt(frame)
	if err != nil {
		return err
	}
	epoch := binary.BigEndian.Uint16(frame[:chacha20.TCPEpochPrefixSize])
	peer.ObservePeerEpoch(epoch)
	if handled, err := s.handleService(peer, sender, epoch, plaintext); handled {
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

func (s *Server) runTun() error {
	var frame [settings.DefaultEthernetMTU]byte
	for {
		if s.ctx.Err() != nil {
			return nil
		}
		n, err := s.tun.Read(frame[:])
		if err != nil {
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				continue
			}
			return err
		}
		if n == 0 {
			continue
		}
		destination, ok := ip.ExtractDestIP(frame[:n])
		if !ok {
			continue
		}
		peer, err := s.peers.FindByDestinationIP(destination)
		if err != nil {
			continue
		}
		err = peer.Send(frame[:n])
		switch {
		case err == nil:
		case errors.Is(err, session.ErrPeerClosed):
		case errors.Is(err, session.ErrTransportWrite), errors.Is(err, errSendQueueFull):
			slog.Debug("failed to send packet to peer", "peer", peer.ExternalAddrPort(), "err", err)
		default:
			slog.Warn("dropping peer after send failure", "peer", peer.ExternalAddrPort(), "err", err)
			s.peers.Delete(peer)
		}
	}
}

func (s *Server) handleService(peer *session.Peer, sender *peerSender, carrierEpoch uint16, plaintext []byte) (bool, error) {
	response, epoch, rekeyed, err := peer.HandleRekey(carrierEpoch, &s.deriver, plaintext)
	if err != nil {
		if errors.Is(err, chacha20.ErrEpochExhausted) {
			return true, s.sendControl(sender, servicepacket.EpochExhausted, nil, nil)
		}
		return true, nil
	}
	if rekeyed {
		return true, sender.SendControl(response, func() {
			peer.ActivateSendEpoch(epoch)
		})
	}
	kind, ok := servicepacket.Parse(plaintext)
	if !ok {
		return false, nil
	}
	switch kind {
	case servicepacket.Ping:
		return true, s.sendControl(sender, servicepacket.Pong, nil, nil)
	}
	return true, nil
}

func (s *Server) sendControl(
	sender *peerSender,
	t servicepacket.HeaderType,
	body []byte, after func(),
) error {
	data := make([]byte, 3+len(body))
	copy(data[3:], body)
	if err := servicepacket.Encode(t, data); err != nil {
		return err
	}
	return sender.SendControl(data, after)
}
