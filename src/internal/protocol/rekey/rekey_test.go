package rekey

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"tungo/internal/protocol/chacha20/rekey"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	transport "tungo/internal/transport/tcp"
)

type rekeyTestEpochManager struct {
	nextEpoch uint16
}

type testPeer struct {
	PublicKey []byte
	Enabled   bool
	ClientID  int
}

type testPeers map[string]testPeer

func newTestAllowedPeers(peers []testPeer) testPeers {
	lookup := make(testPeers, len(peers))
	for _, peer := range peers {
		lookup[string(peer.PublicKey)] = peer
	}
	return lookup
}

func (p testPeers) Lookup(publicKey []byte) (int, bool, bool) {
	peer, found := p[string(publicKey)]
	return peer.ClientID, peer.Enabled, found
}

func (r *rekeyTestEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (*rekeyTestEpochManager) PromoteSendEpoch(uint16)   {}
func (*rekeyTestEpochManager) RetirePreviousEpoch() bool { return true }

func buildRekeyInitPacket(t *testing.T, crypto keys.KeyDeriver) ([]byte, [32]byte) {
	t.Helper()
	pub, priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, v1PacketLen)
	if err := servicepacket.Encode(servicepacket.RekeyInit, pkt); err != nil {
		t.Fatal(err)
	}
	copy(pkt[3:], pub)
	return pkt, priv
}

func seedPendingClientRekey(t *testing.T, coordinator *ClientRekeyCoordinator) {
	t.Helper()
	_, ok, err := coordinator.MaybeBuildRekeyInit(
		coordinator.rotateAt.Add(time.Second), make([]byte, v1PacketLen),
	)
	if err != nil || !ok {
		t.Fatalf("seed pending client rekey: ok=%v err=%v", ok, err)
	}
}

func handleServerRekeyInit(
	crypto keys.KeyDeriver,
	controller epochController,
	packet []byte,
) ([]byte, uint16, bool, error) {
	return NewServerRekeyCoordinator(controller, nil).Handle(0, crypto, packet)
}

func TestServerRekeyCoordinator_Handle_NilController(t *testing.T) {
	_, _, ok, err := handleServerRekeyInit(&keys.DefaultKeyDeriver{}, nil, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil controller, got ok=%v err=%v", ok, err)
	}
}

func TestServerRekeyCoordinator_Handle_NilCrypto(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	_, _, ok, err := handleServerRekeyInit(nil, fsm, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestServerRekeyCoordinator_Handle_ShortPacket(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	_, _, ok, err := handleServerRekeyInit(&keys.DefaultKeyDeriver{}, fsm, make([]byte, 10))
	if err != nil || ok {
		t.Fatalf("expected ok=false for short packet, got ok=%v err=%v", ok, err)
	}
}

func TestServerRekeyCoordinator_Handle_NotReady(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	// Make the FSM unavailable by starting a rekey.
	_, _ = fsm.StartRekey(make([]byte, 32), make([]byte, 32))

	crypto := &keys.DefaultKeyDeriver{}
	pkt, _ := buildRekeyInitPacket(t, crypto)

	_, _, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if err != nil || ok {
		t.Fatalf("expected ok=false when FSM is not ready, got ok=%v err=%v", ok, err)
	}
}

func TestServerRekeyCoordinator_Handle_Success(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	pkt, _ := buildRekeyInitPacket(t, crypto)

	serverPub, epoch, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(serverPub) != v1PacketLen || bytes.Equal(serverPub[serviceHeaderLen:], make([]byte, v1PublicKeyLen)) {
		t.Fatal("expected non-zero server public key")
	}
	if epoch == 0 {
		t.Fatal("expected non-zero epoch")
	}
}

func TestRekeyV2(t *testing.T) {
	crypto := &keys.DefaultKeyDeriver{}
	serverPub, serverPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientPub, clientPriv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverHandshake := noise.NewIKHandshakeServer(
		serverPub[:],
		serverPriv[:],
		newTestAllowedPeers([]testPeer{{
			PublicKey: clientPub[:],
			Enabled:   true,
			ClientID:  1,
		}}),
		nil,
		nil,
	)
	clientHandshake := noise.NewIKHandshakeClient(clientPub[:], clientPriv[:], serverPub[:])

	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()
	clientTransport, err := transport.NewFramedConn(clientConn, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, err := transport.NewFramedConn(serverConn, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverResult := make(chan error, 1)
	go func() {
		_, handshakeErr := serverHandshake.ServerSideHandshake(serverTransport)
		serverResult <- handshakeErr
	}()
	if err := clientHandshake.ClientSideHandshake(clientTransport); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}

	initialC2S := append([]byte(nil), clientHandshake.KeyClientToServer()...)
	initialS2C := append([]byte(nil), clientHandshake.KeyServerToClient()...)
	clientEpochs := &rekeyTestEpochManager{}
	serverEpochs := &rekeyTestEpochManager{}
	clientController := rekey.NewStateMachine(clientEpochs, initialC2S, initialS2C)
	serverController := rekey.NewStateMachine(serverEpochs, initialC2S, initialS2C)
	clientCoordinator := NewClientRekeyCoordinator(
		crypto,
		clientController,
		clientHandshake,
		time.Millisecond,
		time.Now().Add(-time.Second),
	)
	serverCoordinator := NewServerRekeyCoordinator(serverController, serverHandshake)

	init, ok, err := clientCoordinator.MaybeBuildRekeyInit(time.Now(), make([]byte, 1500))
	if err != nil || !ok {
		t.Fatalf("build RekeyInitV2: ok=%v err=%v", ok, err)
	}
	if kind, parsed := servicepacket.Parse(init); !parsed || kind != servicepacket.RekeyInitV2 {
		t.Fatalf("unexpected init type: kind=%v parsed=%v", kind, parsed)
	}

	ack, serverEpoch, ok, err := serverCoordinator.Handle(0, crypto, init)
	if err != nil || !ok {
		t.Fatalf("handle RekeyInitV2: ok=%v err=%v", ok, err)
	}
	repeatedAck, repeatedEpoch, ok, err := serverCoordinator.Handle(0, crypto, init)
	if err != nil || !ok || repeatedEpoch != serverEpoch || !bytes.Equal(repeatedAck, ack) {
		t.Fatalf("repeated init changed transaction: ok=%v epoch=%d err=%v", ok, repeatedEpoch, err)
	}
	if serverEpochs.nextEpoch != 1 {
		t.Fatalf("repeated init staged %d epochs", serverEpochs.nextEpoch)
	}

	if kind, parsed := servicepacket.Parse(ack); !parsed || kind != servicepacket.RekeyAckV2 {
		t.Fatalf("unexpected ack type: kind=%v parsed=%v", kind, parsed)
	}
	if ok, err := clientCoordinator.HandleRekeyAck(0, ack); err != nil || !ok {
		t.Fatalf("handle RekeyAckV2: ok=%v err=%v", ok, err)
	}
	serverCoordinator.ActivateSendEpoch(serverEpoch)

	clientC2S, clientS2C := clientController.CurrentKeys()
	serverC2S, serverS2C := serverController.CurrentKeys()
	if !bytes.Equal(clientC2S, serverC2S) || !bytes.Equal(clientS2C, serverS2C) {
		t.Fatal("client and server installed different V2 keys")
	}
	if bytes.Equal(clientC2S, initialC2S) || bytes.Equal(clientS2C, initialS2C) {
		t.Fatal("V2 did not replace the initial traffic keys")
	}

	v1Packet, _ := buildRekeyInitPacket(t, crypto)
	if _, _, ok, err := serverCoordinator.Handle(1, crypto, v1Packet); err != nil || ok {
		t.Fatalf("negotiated V2 accepted downgrade: ok=%v err=%v", ok, err)
	}
}

func TestServerRekeyCoordinator_RepeatedInitReturnsSameTransaction(t *testing.T) {
	router := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := NewServerRekeyCoordinator(fsm, nil)
	packet, _ := buildRekeyInitPacket(t, crypto)

	firstPub, firstEpoch, firstOK, err := coordinator.Handle(0, crypto, packet)
	if err != nil || !firstOK {
		t.Fatalf("first init: ok=%v err=%v", firstOK, err)
	}
	secondPub, secondEpoch, secondOK, err := coordinator.Handle(0, crypto, packet)
	if err != nil || !secondOK {
		t.Fatalf("repeated init: ok=%v err=%v", secondOK, err)
	}
	if !bytes.Equal(firstPub, secondPub) || firstEpoch != secondEpoch {
		t.Fatal("repeated init returned a different transaction")
	}
	if router.nextEpoch != 1 {
		t.Fatalf("expected one staged epoch, got %d", router.nextEpoch)
	}
}

func TestServerRekeyCoordinator_ConcurrentRepeatedInitStagesOnce(t *testing.T) {
	router := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := NewServerRekeyCoordinator(fsm, nil)
	packet, _ := buildRekeyInitPacket(t, crypto)

	type result struct {
		serverPub []byte
		epoch     uint16
		ok        bool
		err       error
	}
	results := make([]result, 16)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i].serverPub, results[i].epoch, results[i].ok, results[i].err = coordinator.Handle(0, crypto, packet)
		}()
	}
	wg.Wait()

	for i := range results {
		if results[i].err != nil || !results[i].ok {
			t.Fatalf("result %d: ok=%v err=%v", i, results[i].ok, results[i].err)
		}
		if i > 0 && (!bytes.Equal(results[i].serverPub, results[0].serverPub) || results[i].epoch != results[0].epoch) {
			t.Fatalf("result %d returned a different transaction", i)
		}
	}
	if router.nextEpoch != 1 {
		t.Fatalf("expected one staged epoch, got %d", router.nextEpoch)
	}
}

func TestServerRekeyCoordinator_RejectsDifferentInitUntilTransitionCompletes(t *testing.T) {
	router := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := NewServerRekeyCoordinator(fsm, nil)
	firstPacket, _ := buildRekeyInitPacket(t, crypto)
	secondPacket, _ := buildRekeyInitPacket(t, crypto)

	_, epoch, ok, err := coordinator.Handle(0, crypto, firstPacket)
	if err != nil || !ok {
		t.Fatalf("first init: ok=%v err=%v", ok, err)
	}
	if _, _, ok, err = coordinator.Handle(0, crypto, secondPacket); err != nil || ok {
		t.Fatalf("different in-flight init: ok=%v err=%v", ok, err)
	}

	coordinator.ActivateSendEpoch(epoch)
	if _, _, ok, err = coordinator.Handle(0, crypto, secondPacket); err != nil || ok {
		t.Fatalf("init before peer confirmation: ok=%v err=%v", ok, err)
	}

	coordinator.ObservePeerEpoch(epoch)
	if _, _, ok, err = coordinator.Handle(0, crypto, firstPacket); err != nil || ok {
		t.Fatalf("completed transaction replay: ok=%v err=%v", ok, err)
	}
	_, nextEpoch, ok, err := coordinator.Handle(epoch, crypto, secondPacket)
	if err != nil || !ok {
		t.Fatalf("init after completed transition: ok=%v err=%v", ok, err)
	}
	if nextEpoch != epoch+1 {
		t.Fatalf("expected epoch %d, got %d", epoch+1, nextEpoch)
	}
}

func TestServerRekeyCoordinator_RejectsStaleInitAcrossTransactions(t *testing.T) {
	router := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := NewServerRekeyCoordinator(fsm, nil)
	firstPacket, _ := buildRekeyInitPacket(t, crypto)
	secondPacket, _ := buildRekeyInitPacket(t, crypto)
	thirdPacket, _ := buildRekeyInitPacket(t, crypto)

	_, firstEpoch, ok, err := coordinator.Handle(0, crypto, firstPacket)
	if err != nil || !ok {
		t.Fatalf("first init: ok=%v err=%v", ok, err)
	}
	coordinator.ActivateSendEpoch(firstEpoch)
	coordinator.ObservePeerEpoch(firstEpoch)

	_, secondEpoch, ok, err := coordinator.Handle(firstEpoch, crypto, secondPacket)
	if err != nil || !ok {
		t.Fatalf("second init: ok=%v err=%v", ok, err)
	}
	coordinator.ActivateSendEpoch(secondEpoch)
	coordinator.ObservePeerEpoch(secondEpoch)

	if _, _, ok, err = coordinator.Handle(0, crypto, firstPacket); err != nil || ok {
		t.Fatalf("stale first init: ok=%v err=%v", ok, err)
	}
	if router.nextEpoch != secondEpoch {
		t.Fatalf("stale init staged epoch %d, want %d", router.nextEpoch, secondEpoch)
	}

	_, thirdEpoch, ok, err := coordinator.Handle(secondEpoch, crypto, thirdPacket)
	if err != nil || !ok {
		t.Fatalf("third init after stale packet: ok=%v err=%v", ok, err)
	}
	if thirdEpoch != secondEpoch+1 {
		t.Fatalf("third epoch = %d, want %d", thirdEpoch, secondEpoch+1)
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_NilController(t *testing.T) {
	coordinator := NewClientRekeyCoordinator(&keys.DefaultKeyDeriver{}, nil, nil, time.Hour, time.Now())
	ok, err := coordinator.HandleRekeyAck(0, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil controller, got ok=%v err=%v", ok, err)
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_NilCrypto(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	coordinator := NewClientRekeyCoordinator(nil, fsm, nil, time.Hour, time.Now())
	ok, err := coordinator.HandleRekeyAck(0, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_ShortPacket(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	coordinator := NewClientRekeyCoordinator(&keys.DefaultKeyDeriver{}, fsm, nil, time.Hour, time.Now())
	ok, err := coordinator.HandleRekeyAck(0, make([]byte, 10))
	if err != nil || ok {
		t.Fatalf("expected ok=false for short packet, got ok=%v err=%v", ok, err)
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_NoPendingKey(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))

	pkt := make([]byte, v1PacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, pkt)

	coordinator := NewClientRekeyCoordinator(&keys.DefaultKeyDeriver{}, fsm, nil, time.Hour, time.Now())
	ok, err := coordinator.HandleRekeyAck(0, pkt)
	if err != nil || ok {
		t.Fatalf("expected ok=false without pending key, got ok=%v err=%v", ok, err)
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_Success(t *testing.T) {
	rk := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	coordinator := NewClientRekeyCoordinator(crypto, fsm, nil, time.Hour, time.Now())
	seedPendingClientRekey(t, coordinator)

	// Build an ack packet with a server public key.
	serverPub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, v1PacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, pkt)
	copy(pkt[3:], serverPub)

	ok, err := coordinator.HandleRekeyAck(0, pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Pending key should be cleared.
	if coordinator.hasPendingPrivateKey {
		t.Fatal("expected pending key to be cleared after ack")
	}
}

func TestClientRekeyCoordinator_RejectsStaleAckAcrossTransactions(t *testing.T) {
	router := &rekeyTestEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := NewClientRekeyCoordinator(crypto, fsm, nil, time.Hour, time.Now())

	seedPendingClientRekey(t, coordinator)
	firstAck := buildRekeyAckPacket(t, crypto)
	if ok, err := coordinator.HandleRekeyAck(0, firstAck); err != nil || !ok {
		t.Fatalf("first ack: ok=%v err=%v", ok, err)
	}
	fsm.ObservePeerEpoch(1)

	seedPendingClientRekey(t, coordinator)
	if ok, err := coordinator.HandleRekeyAck(0, firstAck); err != nil || ok {
		t.Fatalf("stale first ack: ok=%v err=%v", ok, err)
	}
	if !coordinator.hasPendingPrivateKey {
		t.Fatal("stale ack cleared the second transaction")
	}
	if got := fsm.SendEpoch(); got != 1 {
		t.Fatalf("stale ack changed send epoch to %d, want 1", got)
	}

	secondAck := buildRekeyAckPacket(t, crypto)
	if ok, err := coordinator.HandleRekeyAck(1, secondAck); err != nil || !ok {
		t.Fatalf("second ack: ok=%v err=%v", ok, err)
	}
	if got := fsm.SendEpoch(); got != 2 {
		t.Fatalf("second ack changed send epoch to %d, want 2", got)
	}
}

func buildRekeyAckPacket(t *testing.T, crypto keys.KeyDeriver) []byte {
	t.Helper()
	serverPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, v1PacketLen)
	if err := servicepacket.Encode(servicepacket.RekeyAck, packet); err != nil {
		t.Fatal(err)
	}
	copy(packet[3:], serverPub)
	return packet
}

// mockCrypto is a controllable mock of keys.KeyDeriver for testing error paths.
type mockCrypto struct {
	genPub    []byte
	genPriv   [32]byte
	genErr    error
	deriveErr error
	deriveN   int // 1-based: which call to DeriveKey should fail; 0 = never
	deriveCnt int
}

func (f *mockCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	if f.genErr != nil {
		return nil, f.genPriv, f.genErr
	}
	if f.genPub != nil {
		return f.genPub, f.genPriv, nil
	}
	return (&keys.DefaultKeyDeriver{}).GenerateX25519KeyPair()
}
func (f *mockCrypto) DeriveKey(_, _, _ []byte) ([]byte, error) {
	f.deriveCnt++
	if f.deriveN > 0 && f.deriveCnt == f.deriveN {
		return nil, f.deriveErr
	}
	return make([]byte, 32), nil
}

func TestServerRekeyCoordinator_Handle_GenerateKeyPairError(t *testing.T) {
	genErr := errors.New("keygen failed")
	crypto := &mockCrypto{genErr: genErr}
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	pkt, _ := buildRekeyInitPacket(t, &keys.DefaultKeyDeriver{})

	_, _, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if !errors.Is(err, genErr) {
		t.Fatalf("expected keygen error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestServerRekeyCoordinator_Handle_DeriveKeyError_FirstCall(t *testing.T) {
	deriveErr := errors.New("derive c2s failed")
	crypto := &mockCrypto{deriveErr: deriveErr, deriveN: 1}
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	pkt, _ := buildRekeyInitPacket(t, &keys.DefaultKeyDeriver{})

	_, _, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if !errors.Is(err, deriveErr) {
		t.Fatalf("expected derive error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestServerRekeyCoordinator_Handle_DeriveKeyError_SecondCall(t *testing.T) {
	deriveErr := errors.New("derive s2c failed")
	crypto := &mockCrypto{deriveErr: deriveErr, deriveN: 2}
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	pkt, _ := buildRekeyInitPacket(t, &keys.DefaultKeyDeriver{})

	_, _, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if !errors.Is(err, deriveErr) {
		t.Fatalf("expected derive error on second call, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestServerRekeyCoordinator_Handle_WrongSizePublicKey(t *testing.T) {
	crypto := &mockCrypto{genPub: make([]byte, 31)} // wrong size
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	pkt, _ := buildRekeyInitPacket(t, &keys.DefaultKeyDeriver{})

	_, _, ok, err := handleServerRekeyInit(crypto, fsm, pkt)
	if err == nil {
		t.Fatal("expected error for wrong-size public key")
	}
	if ok {
		t.Fatal("expected ok=false for wrong-size public key")
	}
}

// mockRekeyController implements serverRekeyController with a controllable StartRekey error.
type mockRekeyController struct {
	ready    bool
	c2sKey   []byte
	s2cKey   []byte
	startErr error
}

func (m *mockRekeyController) ReadyForRekey() bool                    { return m.ready }
func (m *mockRekeyController) SendEpoch() uint16                      { return 0 }
func (m *mockRekeyController) StartRekey(_, _ []byte) (uint16, error) { return 0, m.startErr }
func (m *mockRekeyController) ActivateSendEpoch(uint16)               {}
func (m *mockRekeyController) ObservePeerEpoch(uint16)                {}
func (m *mockRekeyController) CurrentKeys() ([]byte, []byte) {
	return append([]byte(nil), m.c2sKey...), append([]byte(nil), m.s2cKey...)
}

func TestServerRekeyCoordinator_Handle_StartRekeyError(t *testing.T) {
	crypto := &keys.DefaultKeyDeriver{}
	controller := &mockRekeyController{
		ready:    true,
		c2sKey:   make([]byte, 32),
		s2cKey:   make([]byte, 32),
		startErr: errors.New("rekey-fail"),
	}
	pkt, _ := buildRekeyInitPacket(t, crypto)

	_, _, ok, err := handleServerRekeyInit(crypto, controller, pkt)
	if err == nil {
		t.Fatal("expected StartRekey error")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_StartRekeyError(t *testing.T) {
	// Use an epoch manager that always fails.
	rk := &failingEpochManager{}
	crypto := &keys.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	coordinator := NewClientRekeyCoordinator(crypto, fsm, nil, time.Hour, time.Now())
	seedPendingClientRekey(t, coordinator)

	serverPub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, v1PacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, pkt)
	copy(pkt[3:], serverPub)

	ok, err := coordinator.HandleRekeyAck(0, pkt)
	if err == nil {
		t.Fatal("expected StartRekey error from failing epoch manager")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

// failingEpochManager always returns an error from Rekey.
type failingEpochManager struct{}

func (*failingEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	return 0, errors.New("rekey-fail")
}
func (*failingEpochManager) PromoteSendEpoch(uint16)   {}
func (*failingEpochManager) RetirePreviousEpoch() bool { return true }

func TestClientRekeyCoordinator_HandleRekeyAck_DeriveKeyError_FirstCall(t *testing.T) {
	deriveErr := errors.New("derive c2s failed")
	crypto := &mockCrypto{deriveErr: deriveErr, deriveN: 1}
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	realCrypto := &keys.DefaultKeyDeriver{}
	coordinator := NewClientRekeyCoordinator(crypto, fsm, nil, time.Hour, time.Now())
	seedPendingClientRekey(t, coordinator)

	serverPub, _, _ := realCrypto.GenerateX25519KeyPair()
	pkt := make([]byte, v1PacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, pkt)
	copy(pkt[3:], serverPub)

	ok, err := coordinator.HandleRekeyAck(0, pkt)
	if !errors.Is(err, deriveErr) {
		t.Fatalf("expected derive error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestClientRekeyCoordinator_HandleRekeyAck_DeriveKeyError_SecondCall(t *testing.T) {
	deriveErr := errors.New("derive s2c failed")
	crypto := &mockCrypto{deriveErr: deriveErr, deriveN: 2}
	rk := &rekeyTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))

	realCrypto := &keys.DefaultKeyDeriver{}
	coordinator := NewClientRekeyCoordinator(crypto, fsm, nil, time.Hour, time.Now())
	seedPendingClientRekey(t, coordinator)

	serverPub, _, _ := realCrypto.GenerateX25519KeyPair()
	pkt := make([]byte, v1PacketLen)
	_ = servicepacket.Encode(servicepacket.RekeyAck, pkt)
	copy(pkt[3:], serverPub)

	ok, err := coordinator.HandleRekeyAck(0, pkt)
	if !errors.Is(err, deriveErr) {
		t.Fatalf("expected derive error on second call, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}
