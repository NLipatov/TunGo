package rekey

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// StateMachineRekeyerMock is a controllable mock for Rekeyer.
// Name is prefixed with the tested structure per convention.
type StateMachineRekeyerMock struct {
	mu sync.Mutex

	// Rekey behavior.
	rekeyEpoch uint16
	rekeyErr   error

	// Optional blocking to simulate long crypto work and interleavings.
	rekeyEntered chan struct{}   // closed when Rekey is entered
	rekeyBlock   <-chan struct{} // if non-nil, Rekey waits until it's closed

	// Call records.
	rekeyCalls int
	rekeySend  [][]byte
	rekeyRecv  [][]byte

	setSendEpochCalls []uint16
	removeEpochCalls  []uint16

	removeEpochReturn bool
}

func (m *StateMachineRekeyerMock) Rekey(sendKey, recvKey []byte) (uint16, error) {
	// Record arguments as copies to avoid aliasing.
	sendCopy := append([]byte(nil), sendKey...)
	recvCopy := append([]byte(nil), recvKey...)

	m.mu.Lock()
	m.rekeyCalls++
	m.rekeySend = append(m.rekeySend, sendCopy)
	m.rekeyRecv = append(m.rekeyRecv, recvCopy)
	entered := m.rekeyEntered
	block := m.rekeyBlock
	epoch := m.rekeyEpoch
	err := m.rekeyErr
	m.mu.Unlock()

	if entered != nil {
		select {
		case <-entered:
			// already closed
		default:
			close(entered)
		}
	}
	if block != nil {
		<-block
	}
	return epoch, err
}

func (m *StateMachineRekeyerMock) SetSendEpoch(epoch uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setSendEpochCalls = append(m.setSendEpochCalls, epoch)
}

func (m *StateMachineRekeyerMock) RemoveEpoch(epoch uint16) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeEpochCalls = append(m.removeEpochCalls, epoch)
	// Default to true unless explicitly set otherwise.
	if m.removeEpochCalls != nil && !m.removeEpochReturn && m.removeEpochReturn != false {
		// no-op; keep explicitness simple
	}
	return m.removeEpochReturn || m.removeEpochReturn == false // allow default false only if set
}

func (m *StateMachineRekeyerMock) Snapshot() (rekeyCalls int, rekeySend, rekeyRecv [][]byte, setCalls, removeCalls []uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Deep-ish copy slices for safety in asserts.
	rekeySend = make([][]byte, len(m.rekeySend))
	rekeyRecv = make([][]byte, len(m.rekeyRecv))
	for i := range m.rekeySend {
		rekeySend[i] = append([]byte(nil), m.rekeySend[i]...)
		rekeyRecv[i] = append([]byte(nil), m.rekeyRecv[i]...)
	}
	setCalls = append([]uint16(nil), m.setSendEpochCalls...)
	removeCalls = append([]uint16(nil), m.removeEpochCalls...)
	return m.rekeyCalls, rekeySend, rekeyRecv, setCalls, removeCalls
}

func TestNewStateMachine_InitialStateAndKeyCopies(t *testing.T) {
	mock := &StateMachineRekeyerMock{}
	c2s := []byte{1, 2, 3}
	s2c := []byte{4, 5, 6}

	sm := NewStateMachine(mock, c2s, s2c, true)

	if sm.State() != StateStable {
		t.Fatalf("expected initial state Stable, got %v", sm.State())
	}
	if sm.sendEpoch != 0 {
		t.Fatalf("expected sendEpoch=0, got %d", sm.sendEpoch)
	}

	// Ensure keys are copied on construction.
	gotC2S := sm.CurrentClientToServerKey()
	gotS2C := sm.CurrentServerToClientKey()
	if !reflect.DeepEqual(gotC2S, c2s) || !reflect.DeepEqual(gotS2C, s2c) {
		t.Fatalf("expected keys to match initial values")
	}
	c2s[0] = 9
	s2c[0] = 9
	gotC2S2 := sm.CurrentClientToServerKey()
	gotS2C2 := sm.CurrentServerToClientKey()
	if gotC2S2[0] == 9 || gotS2C2[0] == 9 {
		t.Fatalf("expected internal keys to be independent copies")
	}
}

func TestStartRekey_Success_GoesPendingAndDoesNotSwitchSendUntilAck(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 10}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	// Deterministic time.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sm.SetNowFunc(func() time.Time { return base })

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 10 {
		t.Fatalf("expected epoch=10, got %d", epoch)
	}

	if sm.State() != StatePending {
		t.Fatalf("expected state Pending after StartRekey, got %v", sm.State())
	}
	if !sm.hasPending || sm.pendingSendEpoch != 10 {
		t.Fatalf("expected pending epoch=10, hasPending=%v pending=%d", sm.hasPending, sm.pendingSendEpoch)
	}
	if !sm.pendingSince.Equal(base) {
		t.Fatalf("expected pendingSince=%v, got %v", base, sm.pendingSince)
	}

	// Must not switch send epoch until confirmed.
	if sm.sendEpoch != 0 {
		t.Fatalf("expected sendEpoch still 0, got %d", sm.sendEpoch)
	}

	_, _, _, setCalls, removeCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected SetSendEpoch not called yet, got %v", setCalls)
	}
	if len(removeCalls) != 0 {
		t.Fatalf("expected RemoveEpoch not called, got %v", removeCalls)
	}
}

func TestActivateSendEpoch_NormalPath_PromotesKeysAndBecomesStable(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 7}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if sm.State() != StatePending {
		t.Fatalf("expected Pending, got %v", sm.State())
	}

	sm.ActivateSendEpoch(epoch)

	if sm.State() != StateStable {
		t.Fatalf("expected Stable after activation, got %v", sm.State())
	}
	if sm.sendEpoch != epoch || sm.LastRekeyEpoch != epoch {
		t.Fatalf("expected sendEpoch/LastRekeyEpoch=%d, got send=%d last=%d", epoch, sm.sendEpoch, sm.LastRekeyEpoch)
	}
	if sm.hasPending || sm.pendingSendEpoch != 0 {
		t.Fatalf("expected pending cleared, hasPending=%v pendingSendEpoch=%d", sm.hasPending, sm.pendingSendEpoch)
	}

	// Keys should be promoted.
	if string(sm.CurrentClientToServerKey()) != "new-c2s" || string(sm.CurrentServerToClientKey()) != "new-s2c" {
		t.Fatalf("expected promoted keys new-c2s/new-s2c, got %q/%q", sm.CurrentClientToServerKey(), sm.CurrentServerToClientKey())
	}

	_, _, _, setCalls, _ := mock.Snapshot()
	if len(setCalls) != 1 || setCalls[0] != epoch {
		t.Fatalf("expected SetSendEpoch(%d) once, got %v", epoch, setCalls)
	}
}

func TestActivateSendEpoch_DoesNotActivateIfEpochNotConfirmed(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 10}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 10 {
		t.Fatalf("expected epoch=10, got %d", epoch)
	}

	// Confirm a smaller epoch than pending; should not activate.
	sm.ActivateSendEpoch(9)

	if sm.State() != StatePending {
		t.Fatalf("expected still Pending, got %v", sm.State())
	}
	if sm.sendEpoch != 0 {
		t.Fatalf("expected sendEpoch still 0, got %d", sm.sendEpoch)
	}

	_, _, _, setCalls, _ := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no SetSendEpoch calls, got %v", setCalls)
	}
}

func TestStartRekey_EarlyAckDuringRekeying_AutoActivatesOnReturn(t *testing.T) {
	rekeyEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineRekeyerMock{
		rekeyEpoch:   42,
		rekeyEntered: rekeyEntered,
		rekeyBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	done := make(chan struct{})
	var startEpoch uint16
	var startErr error
	go func() {
		defer close(done)
		startEpoch, startErr = sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	}()

	// Wait until crypto.Rekey is entered and StartRekey has released the mutex.
	select {
	case <-rekeyEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	// Early ACK while state is expected to be StateRekeying.
	sm.ActivateSendEpoch(42)

	// Let crypto.Rekey return.
	close(rekeyUnblock)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartRekey to finish")
	}

	if startErr != nil {
		t.Fatalf("StartRekey unexpected error: %v", startErr)
	}
	if startEpoch != 42 {
		t.Fatalf("expected epoch=42, got %d", startEpoch)
	}

	// Should have auto-activated and ended Stable (early-ack fast-forward).
	if sm.State() != StateStable {
		t.Fatalf("expected Stable after early-ack activation, got %v", sm.State())
	}
	if sm.sendEpoch != 42 || sm.LastRekeyEpoch != 42 {
		t.Fatalf("expected sendEpoch/LastRekeyEpoch=42, got send=%d last=%d", sm.sendEpoch, sm.LastRekeyEpoch)
	}
	if string(sm.CurrentClientToServerKey()) != "new-c2s" || string(sm.CurrentServerToClientKey()) != "new-s2c" {
		t.Fatalf("expected promoted keys new-c2s/new-s2c, got %q/%q", sm.CurrentClientToServerKey(), sm.CurrentServerToClientKey())
	}

	_, _, _, setCalls, _ := mock.Snapshot()
	if len(setCalls) != 1 || setCalls[0] != 42 {
		t.Fatalf("expected SetSendEpoch(42) once, got %v", setCalls)
	}
}

func TestAbortPendingIfExpired_NoOpBeforeTimeout(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 5}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	sm.SetNowFunc(func() time.Time { return base })
	sm.SetPendingTimeout(10 * time.Second)

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 5 {
		t.Fatalf("expected epoch=5, got %d", epoch)
	}

	sm.AbortPendingIfExpired(base.Add(9 * time.Second))

	if sm.State() != StatePending {
		t.Fatalf("expected still Pending, got %v", sm.State())
	}
	_, _, _, _, removeCalls := mock.Snapshot()
	if len(removeCalls) != 0 {
		t.Fatalf("expected no RemoveEpoch calls, got %v", removeCalls)
	}
}

func TestAbortPendingIfExpired_AbortsAfterTimeout_RemovesEpochAndResets(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 5}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	sm.SetNowFunc(func() time.Time { return base })
	sm.SetPendingTimeout(10 * time.Second)

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 5 {
		t.Fatalf("expected epoch=5, got %d", epoch)
	}

	sm.AbortPendingIfExpired(base.Add(10 * time.Second))

	if sm.State() != StateStable {
		t.Fatalf("expected Stable after abort, got %v", sm.State())
	}
	if sm.hasPending {
		t.Fatalf("expected pending cleared after abort")
	}
	_, _, _, setCalls, removeCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no SetSendEpoch calls on abort, got %v", setCalls)
	}
	if len(removeCalls) != 1 || removeCalls[0] != 5 {
		t.Fatalf("expected RemoveEpoch(5) once, got %v", removeCalls)
	}
}

func TestStartRekey_NotAllowedWhenNotStable(t *testing.T) {
	rekeyEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineRekeyerMock{
		rekeyEpoch:   1,
		rekeyEntered: rekeyEntered,
		rekeyBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	// First StartRekey blocks in crypto and leaves state at StateRekeying for a while.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = sm.StartRekey([]byte("k1"), []byte("k2"))
	}()

	select {
	case <-rekeyEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	// Second StartRekey must fail while first is in-flight.
	if _, err := sm.StartRekey([]byte("k3"), []byte("k4")); err == nil {
		t.Fatalf("expected error when StartRekey is called in non-stable state")
	}

	// Unblock first call.
	close(rekeyUnblock)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartRekey to finish")
	}

	// Now first call ended in Pending; StartRekey still must fail.
	if sm.State() != StatePending {
		t.Fatalf("expected Pending after first StartRekey, got %v", sm.State())
	}
	if _, err := sm.StartRekey([]byte("k5"), []byte("k6")); err == nil {
		t.Fatalf("expected error when StartRekey is called in Pending state")
	}
}

func TestStartRekey_CryptoError_RollsBackToStable_NoCleanupEpoch(t *testing.T) {
	sentinelErr := errors.New("crypto failure")
	mock := &StateMachineRekeyerMock{rekeyErr: sentinelErr}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	_, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected crypto error, got %v", err)
	}
	if sm.State() != StateStable {
		t.Fatalf("expected Stable after crypto error, got %v", sm.State())
	}

	rekeyCalls, _, _, _, removeCalls := mock.Snapshot()
	if rekeyCalls != 1 {
		t.Fatalf("expected Rekey called once, got %d", rekeyCalls)
	}
	if len(removeCalls) != 0 {
		t.Fatalf("expected no RemoveEpoch call when Rekey returns error (no epoch installed), got %v", removeCalls)
	}
}

func TestStartRekey_EpochExhaustedByLastRekeyEpoch_DoesNotCallCrypto(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 1}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	// Force exhaustion.
	sm.mu.Lock()
	sm.LastRekeyEpoch = maxEpochSafety
	sm.mu.Unlock()

	_, err := sm.StartRekey([]byte("x"), []byte("y"))
	if !errors.Is(err, ErrEpochExhausted) {
		t.Fatalf("expected ErrEpochExhausted, got %v", err)
	}

	rekeyCalls, _, _, _, _ := mock.Snapshot()
	if rekeyCalls != 0 {
		t.Fatalf("expected Rekey not called on exhaustion, got %d", rekeyCalls)
	}
}

func TestStartRekey_EpochExhaustedByReturnedEpoch_CleansUpEpoch(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: maxEpochSafety}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	_, err := sm.StartRekey([]byte("x"), []byte("y"))
	if !errors.Is(err, ErrEpochExhausted) {
		t.Fatalf("expected ErrEpochExhausted, got %v", err)
	}
	if sm.State() != StateStable {
		t.Fatalf("expected Stable after exhaustion cleanup, got %v", sm.State())
	}

	_, _, _, _, removeCalls := mock.Snapshot()
	if len(removeCalls) != 1 || removeCalls[0] != maxEpochSafety {
		t.Fatalf("expected RemoveEpoch(%d) once, got %v", maxEpochSafety, removeCalls)
	}
}

func TestStartRekey_NonMonotonicEpoch_CleansUpEpoch(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 5}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	// Pretend we've already activated epoch 5.
	sm.mu.Lock()
	sm.sendEpoch = 5
	sm.LastRekeyEpoch = 5
	sm.mu.Unlock()

	_, err := sm.StartRekey([]byte("x"), []byte("y"))
	if err == nil {
		t.Fatalf("expected non-monotonic epoch error")
	}
	if sm.State() != StateStable {
		t.Fatalf("expected Stable after failure, got %v", sm.State())
	}

	_, _, _, _, removeCalls := mock.Snapshot()
	if len(removeCalls) != 1 || removeCalls[0] != 5 {
		t.Fatalf("expected RemoveEpoch(5) once, got %v", removeCalls)
	}
}

func TestStartRekey_UnexpectedStateAfterRekey_CleansUpEpoch(t *testing.T) {
	rekeyEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineRekeyerMock{
		rekeyEpoch:   9,
		rekeyEntered: rekeyEntered,
		rekeyBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	done := make(chan struct{})
	var gotErr error
	go func() {
		defer close(done)
		_, gotErr = sm.StartRekey([]byte("x"), []byte("y"))
	}()

	select {
	case <-rekeyEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	// Simulate an unexpected state mutation while StartRekey is blocked in crypto.
	sm.mu.Lock()
	sm.state = StateStable
	sm.mu.Unlock()

	close(rekeyUnblock)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartRekey to finish")
	}

	if gotErr == nil {
		t.Fatalf("expected error due to unexpected state after rekey")
	}
	if sm.State() != StateStable {
		t.Fatalf("expected Stable after cleanup, got %v", sm.State())
	}

	_, _, _, _, removeCalls := mock.Snapshot()
	if len(removeCalls) != 1 || removeCalls[0] != 9 {
		t.Fatalf("expected RemoveEpoch(9) once, got %v", removeCalls)
	}
}

func TestStartRekey_CopiesInputSlices_NoExternalMutationLeak(t *testing.T) {
	rekeyEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineRekeyerMock{
		rekeyEpoch:   3,
		rekeyEntered: rekeyEntered,
		rekeyBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	send := []byte{1, 2, 3}
	recv := []byte{4, 5, 6}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = sm.StartRekey(send, recv)
	}()

	select {
	case <-rekeyEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	// Mutate original slices after StartRekey has passed copies to crypto.Rekey.
	send[0] = 9
	recv[0] = 9

	close(rekeyUnblock)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartRekey to finish")
	}

	_, rekeySend, rekeyRecv, _, _ := mock.Snapshot()
	if len(rekeySend) != 1 || len(rekeyRecv) != 1 {
		t.Fatalf("expected exactly one Rekey call")
	}
	if rekeySend[0][0] != 1 || rekeyRecv[0][0] != 4 {
		t.Fatalf("expected crypto to receive original (copied) values, got send=%v recv=%v", rekeySend[0], rekeyRecv[0])
	}
}

func TestActivateSendEpoch_AlwaysTracksMaxPeerEpoch(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 2}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	sm.ActivateSendEpoch(10)
	sm.ActivateSendEpoch(7)
	sm.ActivateSendEpoch(11)

	if sm.peerEpochSeenMax != 11 {
		t.Fatalf("expected peerEpochSeenMax=11, got %d", sm.peerEpochSeenMax)
	}
}

func TestConcurrent_ActivateAndAbort_NoDeadlock_EndsInValidState(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 100}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), false)

	base := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	sm.SetNowFunc(func() time.Time { return base })
	sm.SetPendingTimeout(1 * time.Second)

	epoch, err := sm.StartRekey([]byte("new-c2s"), []byte("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 100 {
		t.Fatalf("expected epoch=100, got %d", epoch)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Try to confirm while another goroutine tries to abort.
		for i := 0; i < 1000; i++ {
			sm.ActivateSendEpoch(100)
		}
	}()
	go func() {
		defer wg.Done()
		// Try to abort; depending on timing, activation may win.
		for i := 0; i < 1000; i++ {
			sm.AbortPendingIfExpired(base.Add(2 * time.Second))
		}
	}()

	done := make(chan struct{})
	go func() { defer close(done); wg.Wait() }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for concurrent operations to finish (possible deadlock)")
	}

	// Final state should be Stable (either by activation or by abort).
	if sm.State() != StateStable {
		t.Fatalf("expected Stable at end, got %v", sm.State())
	}

	// Invariants: no pending should remain if Stable.
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state == StateStable && sm.hasPending {
		t.Fatalf("invariant violated: Stable but hasPending=true")
	}
}

func TestSetNowFunc_NilIsNoOp(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 1}
	sm := NewStateMachine(mock, []byte("c2s"), []byte("s2c"), false)

	// Should not panic or replace the time source.
	sm.SetNowFunc(nil)

	// Verify the FSM still works (now func is intact).
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sm.SetNowFunc(func() time.Time { return base })
	sm.SetPendingTimeout(1 * time.Second)

	epoch, err := sm.StartRekey([]byte("k1"), []byte("k2"))
	if err != nil {
		t.Fatalf("StartRekey failed: %v", err)
	}
	sm.mu.Lock()
	since := sm.pendingSince
	sm.mu.Unlock()
	if !since.Equal(base) {
		t.Fatalf("expected pendingSince=%v, got %v", base, since)
	}
	_ = epoch
}

func TestIsServer(t *testing.T) {
	server := NewStateMachine(&StateMachineRekeyerMock{}, []byte("c2s"), []byte("s2c"), true)
	client := NewStateMachine(&StateMachineRekeyerMock{}, []byte("c2s"), []byte("s2c"), false)

	if !server.IsServer() {
		t.Fatal("expected IsServer()=true for server")
	}
	if client.IsServer() {
		t.Fatal("expected IsServer()=false for client")
	}
}

func TestPendingRekeyPrivateKey_SetGetClear(t *testing.T) {
	sm := NewStateMachine(&StateMachineRekeyerMock{}, []byte("c2s"), []byte("s2c"), false)

	// Initially no pending key.
	if _, ok := sm.PendingRekeyPrivateKey(); ok {
		t.Fatal("expected no pending key initially")
	}

	// Set a key.
	var key [32]byte
	key[0] = 42
	sm.SetPendingRekeyPrivateKey(key)

	got, ok := sm.PendingRekeyPrivateKey()
	if !ok {
		t.Fatal("expected pending key after set")
	}
	if got[0] != 42 {
		t.Fatalf("expected key[0]=42, got %d", got[0])
	}

	// Clear.
	sm.ClearPendingRekeyPrivateKey()
	if _, ok := sm.PendingRekeyPrivateKey(); ok {
		t.Fatal("expected no pending key after clear")
	}
}

func TestAbortPendingIfExpired_NoOpWhenStable(t *testing.T) {
	mock := &StateMachineRekeyerMock{}
	sm := NewStateMachine(mock, []byte("c2s"), []byte("s2c"), false)

	// Should be a no-op — no panic, no state change.
	sm.AbortPendingIfExpired(time.Now().Add(time.Hour))

	if sm.State() != StateStable {
		t.Fatalf("expected Stable, got %v", sm.State())
	}
}

func TestStartRekey_ServerSideKeyOrientation(t *testing.T) {
	mock := &StateMachineRekeyerMock{rekeyEpoch: 3}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"), true)

	epoch, err := sm.StartRekey([]byte("new-send"), []byte("new-recv"))
	if err != nil {
		t.Fatalf("StartRekey failed: %v", err)
	}

	// Activate to promote keys.
	sm.ActivateSendEpoch(epoch)

	// For isServer=true, sendKey goes to S2C and recvKey goes to C2S.
	if string(sm.CurrentServerToClientKey()) != "new-send" {
		t.Fatalf("expected S2C=new-send, got %q", sm.CurrentServerToClientKey())
	}
	if string(sm.CurrentClientToServerKey()) != "new-recv" {
		t.Fatalf("expected C2S=new-recv, got %q", sm.CurrentClientToServerKey())
	}
}

func TestActivateSendEpoch_NoOpWhenStable(t *testing.T) {
	mock := &StateMachineRekeyerMock{}
	sm := NewStateMachine(mock, []byte("c2s"), []byte("s2c"), false)

	// No pending rekey; ActivateSendEpoch should be a no-op.
	sm.ActivateSendEpoch(5)

	if sm.State() != StateStable {
		t.Fatalf("expected Stable, got %v", sm.State())
	}
	_, _, _, setCalls, _ := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no SetSendEpoch calls, got %v", setCalls)
	}
}
