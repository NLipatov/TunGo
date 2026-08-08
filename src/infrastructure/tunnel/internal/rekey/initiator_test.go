package rekey

import (
	"errors"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
)

type initTestEpochManager struct {
	nextEpoch uint16
}

func (r *initTestEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (r *initTestEpochManager) PromoteSendEpoch(uint16)   {}
func (r *initTestEpochManager) RetirePreviousEpoch() bool { return true }

func TestMaybeBuildRekeyInit_NilCrypto(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	s := NewClientRekeyCoordinator(nil, fsm, time.Second, time.Now().Add(-time.Hour))
	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(time.Now(), dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_NilFSM(t *testing.T) {
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, nil, time.Second, time.Now().Add(-time.Hour))
	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(time.Now(), dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil FSM, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_BeforeRotateAt(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"))
	now := time.Now()
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, fsm, 10*time.Second, now)
	dst := make([]byte, service_packet.RekeyPacketLen)

	// now is before rotateAt (now+10s), should return false.
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(5*time.Second), dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false before rotateAt, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_NotReady(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, fsm, time.Millisecond, now)

	// Make the FSM unavailable for another rekey.
	_, _ = fsm.StartRekey(make([]byte, 32), make([]byte, 32))

	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false when FSM is not ready, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_ShortDst(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, fsm, time.Millisecond, now)

	// dst too short.
	dst := make([]byte, 10)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false for short dst, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_Success(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, fsm, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	payload, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(payload) != service_packet.RekeyPacketLen {
		t.Fatalf("expected payload len=%d, got %d", service_packet.RekeyPacketLen, len(payload))
	}
	// Verify V1 header.
	if payload[0] != service_packet.Prefix || payload[1] != service_packet.VersionV1 || payload[2] != byte(service_packet.RekeyInit) {
		t.Fatalf("unexpected header: %v", payload[:3])
	}
	// Pending private key should be set.
	if !s.hasPendingPrivateKey {
		t.Fatal("expected pending private key to be set")
	}
}

func TestMaybeBuildRekeyInit_SchedulesNextAttempt(t *testing.T) {
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	interval := 5 * time.Second
	s := NewClientRekeyCoordinator(&primitives.DefaultKeyDeriver{}, fsm, interval, now)

	callTime := now.Add(10 * time.Second)
	dst := make([]byte, service_packet.RekeyPacketLen)
	if _, ok, err := s.MaybeBuildRekeyInit(callTime, dst); err != nil || !ok {
		t.Fatalf("first attempt: ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.MaybeBuildRekeyInit(callTime.Add(interval-time.Nanosecond), dst); err != nil || ok {
		t.Fatalf("before next attempt: ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.MaybeBuildRekeyInit(callTime.Add(interval), dst); err != nil || !ok {
		t.Fatalf("next attempt: ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_ReusesPendingKey(t *testing.T) {
	rk := &initTestEpochManager{}
	crypto := &primitives.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(crypto, fsm, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	payload1, ok1, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if err != nil || !ok1 {
		t.Fatalf("first call: ok=%v err=%v", ok1, err)
	}
	pub1 := make([]byte, service_packet.RekeyPublicKeyLen)
	copy(pub1, payload1[3:])

	// MaybeBuildRekeyInit does not stage an epoch, so the FSM remains ready.
	// Second call should reuse the pending key and return the same public key.
	dst2 := make([]byte, service_packet.RekeyPacketLen)
	payload2, ok2, err2 := s.MaybeBuildRekeyInit(now.Add(2*time.Second), dst2)
	if err2 != nil || !ok2 {
		t.Fatalf("second call: ok=%v err=%v", ok2, err2)
	}
	pub2 := make([]byte, service_packet.RekeyPublicKeyLen)
	copy(pub2, payload2[3:])

	if string(pub1) != string(pub2) {
		t.Fatal("expected same public key on second call (pending key reuse)")
	}
}

func TestMaybeBuildRekeyInit_WrongSizePublicKey(t *testing.T) {
	crypto := &mockCrypto{genPub: make([]byte, 31)} // wrong size
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(crypto, fsm, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if err != nil {
		t.Fatalf("expected nil error (silent drop), got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for wrong-size public key")
	}
}

func TestMaybeBuildRekeyInit_GenerateKeyPairError(t *testing.T) {
	genErr := errors.New("keygen failed")
	crypto := &mockCrypto{genErr: genErr}
	rk := &initTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	now := time.Now()
	s := NewClientRekeyCoordinator(crypto, fsm, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), dst)
	if !errors.Is(err, genErr) {
		t.Fatalf("expected keygen error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on keygen error")
	}
}
