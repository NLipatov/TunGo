package controlplane

import (
	"testing"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
)

type rekeyTestRekeyer struct {
	nextEpoch uint16
}

func (r *rekeyTestRekeyer) Rekey(_, _ []byte) (uint16, error) {
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (r *rekeyTestRekeyer) SetSendEpoch(uint16)     {}
func (r *rekeyTestRekeyer) RemoveEpoch(uint16) bool { return true }

func buildRekeyInitPacket(t *testing.T, crypto handshake.Crypto) ([]byte, [32]byte) {
	t.Helper()
	pub, priv, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, service_packet.RekeyPacketLen)
	if _, err := service_packet.EncodeV1Header(service_packet.RekeyInit, pkt); err != nil {
		t.Fatal(err)
	}
	copy(pkt[3:], pub)
	return pkt, priv
}

func TestServerHandleRekeyInit_NilFSM(t *testing.T) {
	_, _, ok, err := ServerHandleRekeyInit(&handshake.DefaultCrypto{}, nil, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil FSM, got ok=%v err=%v", ok, err)
	}
}

func TestServerHandleRekeyInit_NilCrypto(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), true)
	_, _, ok, err := ServerHandleRekeyInit(nil, fsm, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestServerHandleRekeyInit_ShortPacket(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), true)
	_, _, ok, err := ServerHandleRekeyInit(&handshake.DefaultCrypto{}, fsm, make([]byte, 10))
	if err != nil || ok {
		t.Fatalf("expected ok=false for short packet, got ok=%v err=%v", ok, err)
	}
}

func TestServerHandleRekeyInit_NotStable(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), true)
	// Put FSM in non-stable state by starting a rekey.
	fsm.StartRekey([]byte("k1"), []byte("k2"))

	crypto := &handshake.DefaultCrypto{}
	pkt, _ := buildRekeyInitPacket(t, crypto)

	_, _, ok, err := ServerHandleRekeyInit(crypto, fsm, pkt)
	if err != nil || ok {
		t.Fatalf("expected ok=false in non-stable state, got ok=%v err=%v", ok, err)
	}
}

func TestServerHandleRekeyInit_Success(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	crypto := &handshake.DefaultCrypto{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), true)

	pkt, _ := buildRekeyInitPacket(t, crypto)

	serverPub, epoch, ok, err := ServerHandleRekeyInit(crypto, fsm, pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(serverPub) != service_packet.RekeyPublicKeyLen {
		t.Fatalf("expected server pub len=%d, got %d", service_packet.RekeyPublicKeyLen, len(serverPub))
	}
	if epoch == 0 {
		t.Fatal("expected non-zero epoch")
	}
}

func TestClientHandleRekeyAck_NilFSM(t *testing.T) {
	ok, err := ClientHandleRekeyAck(&handshake.DefaultCrypto{}, nil, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil FSM, got ok=%v err=%v", ok, err)
	}
}

func TestClientHandleRekeyAck_NilCrypto(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), false)
	ok, err := ClientHandleRekeyAck(nil, fsm, nil)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestClientHandleRekeyAck_ShortPacket(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), false)
	ok, err := ClientHandleRekeyAck(&handshake.DefaultCrypto{}, fsm, make([]byte, 10))
	if err != nil || ok {
		t.Fatalf("expected ok=false for short packet, got ok=%v err=%v", ok, err)
	}
}

func TestClientHandleRekeyAck_NoPendingKey(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), false)

	pkt := make([]byte, service_packet.RekeyPacketLen)
	service_packet.EncodeV1Header(service_packet.RekeyAck, pkt)

	ok, err := ClientHandleRekeyAck(&handshake.DefaultCrypto{}, fsm, pkt)
	if err != nil || ok {
		t.Fatalf("expected ok=false without pending key, got ok=%v err=%v", ok, err)
	}
}

func TestClientHandleRekeyAck_Success(t *testing.T) {
	rk := &rekeyTestRekeyer{}
	crypto := &handshake.DefaultCrypto{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)

	// Set a pending private key (as the client would after sending RekeyInit).
	_, priv, _ := crypto.GenerateX25519KeyPair()
	fsm.SetPendingRekeyPrivateKey(priv)

	// Build an ack packet with a server public key.
	serverPub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, service_packet.RekeyPacketLen)
	service_packet.EncodeV1Header(service_packet.RekeyAck, pkt)
	copy(pkt[3:], serverPub)

	ok, err := ClientHandleRekeyAck(crypto, fsm, pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Pending key should be cleared.
	if _, hasPending := fsm.PendingRekeyPrivateKey(); hasPending {
		t.Fatal("expected pending key to be cleared after ack")
	}
}
