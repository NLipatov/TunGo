package noise

import (
	"bytes"
	"errors"
	"io"
	"net/netip"
	"strings"
	"testing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"

	noiselib "github.com/flynn/noise"
)

type queueTransport struct {
	reads    [][]byte
	writes   [][]byte
	readErr  error
	writeErr error
}

func (t *queueTransport) Read(p []byte) (int, error) {
	if len(t.reads) > 0 {
		msg := t.reads[0]
		t.reads = t.reads[1:]
		copy(p, msg)
		return len(msg), nil
	}
	if t.readErr != nil {
		return 0, t.readErr
	}
	return 0, io.EOF
}

func (t *queueTransport) Write(p []byte) (int, error) {
	if t.writeErr != nil {
		return 0, t.writeErr
	}
	msg := make([]byte, len(p))
	copy(msg, p)
	t.writes = append(t.writes, msg)
	return len(p), nil
}

func (t *queueTransport) Close() error { return nil }

type queueRemoteTransport struct {
	queueTransport
	addr netip.AddrPort
}

func (t *queueRemoteTransport) RemoteAddrPort() netip.AddrPort {
	return t.addr
}

type cookieRetryTransport struct {
	t                  *testing.T
	serverPub          []byte
	serverPriv         []byte
	cookieManager      *CookieManager
	clientIP           netip.Addr
	nextRead           []byte
	writes             int
	cookieReplyOnRetry bool
}

func (t *cookieRetryTransport) Read(p []byte) (int, error) {
	if t.nextRead == nil {
		return 0, io.EOF
	}
	copy(p, t.nextRead)
	n := len(t.nextRead)
	t.nextRead = nil
	return n, nil
}

func (t *cookieRetryTransport) Write(p []byte) (int, error) {
	t.writes++
	msg1WithMAC, err := CheckVersion(p)
	if err != nil {
		t.t.Fatalf("unexpected version parse error in test transport: %v", err)
	}

	clientEphemeral := ExtractClientEphemeral(msg1WithMAC)
	if clientEphemeral == nil {
		t.t.Fatal("expected client ephemeral in msg1")
	}

	if t.writes == 1 || (t.writes == 2 && t.cookieReplyOnRetry) {
		reply, err := t.cookieManager.CreateCookieReply(t.clientIP, clientEphemeral, t.serverPub)
		if err != nil {
			t.t.Fatalf("failed to create cookie reply: %v", err)
		}
		t.nextRead = reply
		return len(p), nil
	}

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite: cipherSuite,
		Pattern:     noiselib.HandshakeIK,
		Initiator:   false,
		StaticKeypair: noiselib.DHKey{
			Private: t.serverPriv,
			Public:  t.serverPub,
		},
	})
	if err != nil {
		t.t.Fatalf("failed to create server handshake state: %v", err)
	}

	if _, _, _, err = hs.ReadMessage(nil, ExtractNoiseMsg(msg1WithMAC)); err != nil {
		t.t.Fatalf("server read msg1 failed in test transport: %v", err)
	}

	msg2, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		t.t.Fatalf("server write msg2 failed in test transport: %v", err)
	}

	t.nextRead = msg2
	return len(p), nil
}

func (t *cookieRetryTransport) Close() error { return nil }

func newClientMsg1WithVersion(t *testing.T, clientPriv, clientPub, serverPub []byte) []byte {
	t.Helper()
	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite: cipherSuite,
		Pattern:     noiselib.HandshakeIK,
		Initiator:   true,
		StaticKeypair: noiselib.DHKey{
			Private: clientPriv,
			Public:  clientPub,
		},
		PeerStatic: serverPub,
	})
	if err != nil {
		t.Fatalf("failed to create client handshake state: %v", err)
	}
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		t.Fatalf("failed to write msg1: %v", err)
	}
	return PrependVersion(AppendMACs(msg1, serverPub, nil))
}

func TestIKHandshake_Extra_GettersAndNilResult(t *testing.T) {
	var id [32]byte
	copy(id[:], bytes.Repeat([]byte{7}, 32))

	h := &IKHandshake{
		id:        id,
		clientKey: []byte{1, 2, 3},
		serverKey: []byte{4, 5, 6},
	}

	if h.Result() != nil {
		t.Fatal("expected nil result when handshake result not set")
	}
	if h.Id() != id {
		t.Fatal("unexpected handshake ID")
	}
	if !bytes.Equal(h.KeyClientToServer(), []byte{1, 2, 3}) {
		t.Fatal("unexpected client key")
	}
	if !bytes.Equal(h.KeyServerToClient(), []byte{4, 5, 6}) {
		t.Fatal("unexpected server key")
	}

	h.result = &IKHandshakeResult{clientIndex: 9}
	result, ok := h.Result().(*IKHandshakeResult)
	if !ok {
		t.Fatal("expected IKHandshakeResult type")
	}
	if got := result.clientIndex; got != 9 {
		t.Fatalf("expected client index 9, got %d", got)
	}
}

func TestAllowedPeersLookup_EmptyBackingMap(t *testing.T) {
	var lookup allowedPeersMap
	if lookup.Lookup([]byte("missing")) != nil {
		t.Fatal("expected nil when map is not initialized")
	}
}

func TestIKHandshake_Server_ReadErrorAndMissingServerKey(t *testing.T) {
	t.Run("missing server key", func(t *testing.T) {
		h := NewIKHandshakeServer(nil, nil, NewAllowedPeersLookup(nil), nil, nil)
		_, err := h.ServerSideHandshake(&queueTransport{})
		if !errors.Is(err, ErrMissingServerKey) {
			t.Fatalf("expected ErrMissingServerKey, got %v", err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup(nil),
			nil,
			nil,
		)
		_, err := h.ServerSideHandshake(&queueTransport{readErr: io.ErrUnexpectedEOF})
		if err == nil || !strings.Contains(err.Error(), "noise: read msg1") {
			t.Fatalf("expected wrapped read msg1 error, got %v", err)
		}
	})
}

func TestIKHandshake_Server_UnderLoadCookieRequiredBranches(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	cm, _ := NewCookieManager()

	allowedPeers := []server.AllowedPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientIndex: 2},
	}

	msg := newClientMsg1WithVersion(t, clientKP.Private, clientKP.Public, serverKP.Public)

	t.Run("no remote addr under load", func(t *testing.T) {
		lm := NewLoadMonitor(1)
		lm.handshakesPerSecond.Store(2)

		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup(allowedPeers),
			cm,
			lm,
		)

		_, err := h.ServerSideHandshake(&queueTransport{reads: [][]byte{msg}})
		if !errors.Is(err, ErrCookieRequired) {
			t.Fatalf("expected ErrCookieRequired, got %v", err)
		}
	})

	t.Run("cookie reply sent under load", func(t *testing.T) {
		lm := NewLoadMonitor(1)
		lm.handshakesPerSecond.Store(2)

		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup(allowedPeers),
			cm,
			lm,
		)

		tr := &queueRemoteTransport{
			queueTransport: queueTransport{reads: [][]byte{msg}},
			addr:           netip.MustParseAddrPort("203.0.113.10:55000"),
		}

		_, err := h.ServerSideHandshake(tr)
		if !errors.Is(err, ErrCookieRequired) {
			t.Fatalf("expected ErrCookieRequired, got %v", err)
		}
		if len(tr.writes) != 1 {
			t.Fatalf("expected one cookie reply write, got %d", len(tr.writes))
		}
		if !IsCookieReply(tr.writes[0]) {
			t.Fatal("expected cookie reply payload")
		}
	})
}

func TestIKHandshake_Client_CookieRetryPaths(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	cm, _ := NewCookieManager()

	t.Run("retry success", func(t *testing.T) {
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
		tr := &cookieRetryTransport{
			t:             t,
			serverPub:     serverKP.Public,
			serverPriv:    serverKP.Private,
			cookieManager: cm,
			clientIP:      netip.MustParseAddr("198.51.100.20"),
		}

		err := h.ClientSideHandshake(tr, settings.Settings{})
		if err != nil {
			t.Fatalf("unexpected client handshake error: %v", err)
		}
		if len(h.cookie) != CookieSize {
			t.Fatalf("expected cookie size %d after retry, got %d", CookieSize, len(h.cookie))
		}
		if len(h.clientKey) != 32 || len(h.serverKey) != 32 {
			t.Fatal("expected session keys after successful retry")
		}
	})

	t.Run("unexpected cookie on retry", func(t *testing.T) {
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
		tr := &cookieRetryTransport{
			t:                  t,
			serverPub:          serverKP.Public,
			serverPriv:         serverKP.Private,
			cookieManager:      cm,
			clientIP:           netip.MustParseAddr("198.51.100.21"),
			cookieReplyOnRetry: true,
		}

		err := h.ClientSideHandshake(tr, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "unexpected cookie reply on retry") {
			t.Fatalf("expected unexpected-cookie-reply error, got %v", err)
		}
	})
}

func TestIKHandshake_Server_ErrorBranches(t *testing.T) {
	t.Run("under load cookie reply write error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		cm, _ := NewCookieManager()
		lm := NewLoadMonitor(1)
		lm.handshakesPerSecond.Store(2)

		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup([]server.AllowedPeer{
				{PublicKey: clientKP.Public, Enabled: true, ClientIndex: 2},
			}),
			cm,
			lm,
		)

		msg := newClientMsg1WithVersion(t, clientKP.Private, clientKP.Public, serverKP.Public)
		tr := &queueRemoteTransport{
			queueTransport: queueTransport{
				reads:    [][]byte{msg},
				writeErr: io.ErrClosedPipe,
			},
			addr: netip.MustParseAddrPort("203.0.113.50:12345"),
		}

		_, err := h.ServerSideHandshake(tr)
		if err == nil || !strings.Contains(err.Error(), "send cookie reply") {
			t.Fatalf("expected send cookie reply error, got %v", err)
		}
	})

	t.Run("server invalid key fails during msg1 read", func(t *testing.T) {
		serverPub := []byte{1}
		serverPriv := []byte{2}
		noiseMsg := bytes.Repeat([]byte{3}, MinMsg1Size)
		msg := PrependVersion(AppendMACs(noiseMsg, serverPub, nil))

		h := NewIKHandshakeServer(
			serverPub,
			serverPriv,
			NewAllowedPeersLookup(nil),
			nil,
			nil,
		)

		_, err := h.ServerSideHandshake(&queueTransport{reads: [][]byte{msg}})
		if err == nil || !strings.Contains(err.Error(), "read msg1") {
			t.Fatalf("expected read msg1 error, got %v", err)
		}
	})

	t.Run("server read msg1 crypto failure", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		noiseMsg := bytes.Repeat([]byte{0xAA}, MinMsg1Size)
		msg := PrependVersion(AppendMACs(noiseMsg, serverKP.Public, nil))

		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup(nil),
			nil,
			nil,
		)

		_, err := h.ServerSideHandshake(&queueTransport{reads: [][]byte{msg}})
		if err == nil || !strings.Contains(err.Error(), "read msg1") {
			t.Fatalf("expected read msg1 error, got %v", err)
		}
	})

	t.Run("send msg2 write error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		msg := newClientMsg1WithVersion(t, clientKP.Private, clientKP.Public, serverKP.Public)

		h := NewIKHandshakeServer(
			serverKP.Public,
			serverKP.Private,
			NewAllowedPeersLookup([]server.AllowedPeer{
				{PublicKey: clientKP.Public, Enabled: true, ClientIndex: 2},
			}),
			nil,
			nil,
		)

		tr := &queueTransport{
			reads:    [][]byte{msg},
			writeErr: io.ErrClosedPipe,
		}
		_, err := h.ServerSideHandshake(tr)
		if err == nil || !strings.Contains(err.Error(), "send msg2") {
			t.Fatalf("expected send msg2 error, got %v", err)
		}
	})
}

func TestIKHandshake_Client_ErrorBranches(t *testing.T) {
	t.Run("invalid client key fails during msg1 write", func(t *testing.T) {
		h := NewIKHandshakeClient([]byte{1}, []byte{2}, []byte{3})
		err := h.ClientSideHandshake(&queueTransport{}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "write msg1") {
			t.Fatalf("expected write msg1 error, got %v", err)
		}
	})

	t.Run("send msg1 write error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

		err := h.ClientSideHandshake(&queueTransport{writeErr: io.ErrClosedPipe}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "send msg1") {
			t.Fatalf("expected send msg1 error, got %v", err)
		}
	})

	t.Run("cookie decrypt failure", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

		corruptedCookieReply := make([]byte, CookieReplySize)
		err := h.ClientSideHandshake(&queueTransport{reads: [][]byte{corruptedCookieReply}}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "decrypt cookie") {
			t.Fatalf("expected decrypt cookie error, got %v", err)
		}
	})

	t.Run("read msg2 failure", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

		err := h.ClientSideHandshake(&queueTransport{reads: [][]byte{[]byte("bad-msg2")}}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "read msg2") {
			t.Fatalf("expected read msg2 error, got %v", err)
		}
	})
}

func TestIKHandshake_RetryWithCookie_ErrorBranches(t *testing.T) {
	t.Run("retry invalid key fails during msg1 write", func(t *testing.T) {
		h := &IKHandshake{
			clientPubKey:  []byte{1},
			clientPrivKey: []byte{2},
			peerPubKey:    []byte{3},
		}

		err := h.retryWithCookie(&queueTransport{}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "write msg1") {
			t.Fatalf("expected write msg1 error, got %v", err)
		}
	})

	t.Run("retry send msg1 error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
		h.cookie = bytes.Repeat([]byte{1}, CookieSize)

		err := h.retryWithCookie(&queueTransport{writeErr: io.ErrClosedPipe}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "send msg1 retry") {
			t.Fatalf("expected send msg1 retry error, got %v", err)
		}
	})

	t.Run("retry read msg2 error", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
		h.cookie = bytes.Repeat([]byte{1}, CookieSize)

		err := h.retryWithCookie(&queueTransport{readErr: io.ErrUnexpectedEOF}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "read msg2") {
			t.Fatalf("expected read msg2 error, got %v", err)
		}
	})

	t.Run("retry read msg2 crypto failure", func(t *testing.T) {
		serverKP, _ := cipherSuite.GenerateKeypair(nil)
		clientKP, _ := cipherSuite.GenerateKeypair(nil)
		h := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
		h.cookie = bytes.Repeat([]byte{1}, CookieSize)

		err := h.retryWithCookie(&queueTransport{reads: [][]byte{[]byte("bad-msg2")}}, settings.Settings{})
		if err == nil || !strings.Contains(err.Error(), "read msg2") {
			t.Fatalf("expected read msg2 error, got %v", err)
		}
	})
}
