package controlplane

import (
	"errors"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
)

type initTestRekeyer struct {
	nextEpoch uint16
}

func (r *initTestRekeyer) Rekey(_, _ []byte) (uint16, error) {
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (r *initTestRekeyer) SetSendEpoch(uint16)     {}
func (r *initTestRekeyer) RemoveEpoch(uint16) bool { return true }

func TestNewRekeyInitScheduler_Fields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 30 * time.Second
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, interval, now)

	if s.Interval() != interval {
		t.Fatalf("expected interval=%v, got %v", interval, s.Interval())
	}
	if !s.RotateAt().Equal(now.Add(interval)) {
		t.Fatalf("expected rotateAt=%v, got %v", now.Add(interval), s.RotateAt())
	}
}

func TestRekeyInitScheduler_SetInterval(t *testing.T) {
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Second, now)
	s.SetInterval(5 * time.Second)
	if s.Interval() != 5*time.Second {
		t.Fatalf("expected interval=5s, got %v", s.Interval())
	}
}

func TestRekeyInitScheduler_SetRotateAt(t *testing.T) {
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Second, now)
	target := now.Add(10 * time.Second)
	s.SetRotateAt(target)
	if !s.RotateAt().Equal(target) {
		t.Fatalf("expected rotateAt=%v, got %v", target, s.RotateAt())
	}
}

func TestMaybeBuildRekeyInit_NilCrypto(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), false)
	s := NewRekeyInitScheduler(nil, time.Second, time.Now().Add(-time.Hour))
	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(time.Now(), fsm, dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil crypto, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_NilFSM(t *testing.T) {
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Second, time.Now().Add(-time.Hour))
	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(time.Now(), nil, dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false with nil FSM, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_BeforeRotateAt(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), false)
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, 10*time.Second, now)
	dst := make([]byte, service_packet.RekeyPacketLen)

	// now is before rotateAt (now+10s), should return false.
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(5*time.Second), fsm, dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false before rotateAt, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_NotStable(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Millisecond, now)

	// Put FSM in non-stable state.
	_, _ = fsm.StartRekey([]byte("k1"), []byte("k2"))

	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), fsm, dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false when FSM not stable, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_ShortDst(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Millisecond, now)

	// dst too short.
	dst := make([]byte, 10)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), fsm, dst)
	if err != nil || ok {
		t.Fatalf("expected ok=false for short dst, got ok=%v err=%v", ok, err)
	}
}

func TestMaybeBuildRekeyInit_Success(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	payload, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), fsm, dst)
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
	if _, hasPending := fsm.PendingRekeyPrivateKey(); !hasPending {
		t.Fatal("expected pending private key to be set")
	}
}

func TestMaybeBuildRekeyInit_AdvancesRotateAt(t *testing.T) {
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	interval := 5 * time.Second
	s := NewRekeyInitScheduler(&primitives.DefaultKeyDeriver{}, interval, now)

	callTime := now.Add(10 * time.Second)
	dst := make([]byte, service_packet.RekeyPacketLen)
	_, _, _ = s.MaybeBuildRekeyInit(callTime, fsm, dst)

	// rotateAt should have been advanced to callTime + interval.
	expected := callTime.Add(interval)
	if !s.RotateAt().Equal(expected) {
		t.Fatalf("expected rotateAt=%v, got %v", expected, s.RotateAt())
	}
}

func TestMaybeBuildRekeyInit_ReusesPendingKey(t *testing.T) {
	rk := &initTestRekeyer{}
	crypto := &primitives.DefaultKeyDeriver{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	s := NewRekeyInitScheduler(crypto, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	payload1, ok1, err := s.MaybeBuildRekeyInit(now.Add(time.Second), fsm, dst)
	if err != nil || !ok1 {
		t.Fatalf("first call: ok=%v err=%v", ok1, err)
	}
	pub1 := make([]byte, service_packet.RekeyPublicKeyLen)
	copy(pub1, payload1[3:])

	// MaybeBuildRekeyInit does NOT call fsm.StartRekey, so FSM stays Stable.
	// Second call should reuse the pending key and return the same public key.
	dst2 := make([]byte, service_packet.RekeyPacketLen)
	payload2, ok2, err2 := s.MaybeBuildRekeyInit(now.Add(2*time.Second), fsm, dst2)
	if err2 != nil || !ok2 {
		t.Fatalf("second call: ok=%v err=%v", ok2, err2)
	}
	pub2 := make([]byte, service_packet.RekeyPublicKeyLen)
	copy(pub2, payload2[3:])

	if string(pub1) != string(pub2) {
		t.Fatal("expected same public key on second call (pending key reuse)")
	}
}

func TestMaybeBuildRekeyInit_GenerateKeyPairError(t *testing.T) {
	genErr := errors.New("keygen failed")
	crypto := &mockCrypto{genErr: genErr}
	rk := &initTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), false)
	now := time.Now()
	s := NewRekeyInitScheduler(crypto, time.Millisecond, now)

	dst := make([]byte, service_packet.RekeyPacketLen)
	_, ok, err := s.MaybeBuildRekeyInit(now.Add(time.Second), fsm, dst)
	if !errors.Is(err, genErr) {
		t.Fatalf("expected keygen error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on keygen error")
	}
}
