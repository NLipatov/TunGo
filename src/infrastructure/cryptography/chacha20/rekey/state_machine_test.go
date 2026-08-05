package rekey

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// StateMachineEpochManagerMock is a controllable mock for EpochManager.
// Name is prefixed with the tested structure per convention.
type StateMachineEpochManagerMock struct {
	mu sync.Mutex

	// StageEpoch behavior.
	installedEpoch uint16
	installErr     error

	// Optional blocking to simulate long crypto work and interleavings.
	installEntered chan struct{}   // closed when StageEpoch is entered
	installBlock   <-chan struct{} // if non-nil, StageEpoch waits until it's closed

	// Call records.
	installCalls int
	installC2S   [][]byte
	installS2C   [][]byte

	setSendEpochCalls []uint16
	retireEpochCalls  int
	retireEpochFails  bool
}

type noAllocEpochManager struct {
	epoch uint16
}

func (m *noAllocEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	m.epoch++
	return m.epoch, nil
}

func (*noAllocEpochManager) PromoteSendEpoch(uint16) {}

func (*noAllocEpochManager) RetirePreviousEpoch() bool { return true }

func stateMachineTestKey(label string) []byte {
	key := make([]byte, chacha20poly1305.KeySize)
	copy(key, label)
	return key
}

func (m *StateMachineEpochManagerMock) StageEpoch(c2s, s2c []byte) (uint16, error) {
	// Record arguments as copies to avoid aliasing.
	c2sCopy := append([]byte(nil), c2s...)
	s2cCopy := append([]byte(nil), s2c...)

	m.mu.Lock()
	m.installCalls++
	m.installC2S = append(m.installC2S, c2sCopy)
	m.installS2C = append(m.installS2C, s2cCopy)
	entered := m.installEntered
	block := m.installBlock
	epoch := m.installedEpoch
	err := m.installErr
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

func (m *StateMachineEpochManagerMock) PromoteSendEpoch(epoch uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setSendEpochCalls = append(m.setSendEpochCalls, epoch)
}

func (m *StateMachineEpochManagerMock) RetirePreviousEpoch() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retireEpochCalls++
	return !m.retireEpochFails
}

func (m *StateMachineEpochManagerMock) Snapshot() (installCalls int, installC2S, installS2C [][]byte, setCalls []uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Deep-ish copy slices for safety in asserts.
	installC2S = make([][]byte, len(m.installC2S))
	installS2C = make([][]byte, len(m.installS2C))
	for i := range m.installC2S {
		installC2S[i] = append([]byte(nil), m.installC2S[i]...)
		installS2C[i] = append([]byte(nil), m.installS2C[i]...)
	}
	setCalls = append([]uint16(nil), m.setSendEpochCalls...)
	return m.installCalls, installC2S, installS2C, setCalls
}

func (m *StateMachineEpochManagerMock) RetireCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retireEpochCalls
}

func TestNewStateMachine_InitialStateAndKeyCopies(t *testing.T) {
	mock := &StateMachineEpochManagerMock{}
	c2s := []byte{1, 2, 3}
	s2c := []byte{4, 5, 6}

	sm := NewStateMachine(mock, c2s, s2c)

	if !sm.ReadyForRekey() {
		t.Fatal("expected a new state machine to be ready for rekey")
	}
	if sm.state.current.epoch != 0 {
		t.Fatalf("expected sendEpoch=0, got %d", sm.state.current.epoch)
	}

	// Ensure keys are copied on construction.
	gotC2S, gotS2C := sm.CurrentKeys()
	if !reflect.DeepEqual(gotC2S, c2s) || !reflect.DeepEqual(gotS2C, s2c) {
		t.Fatalf("expected keys to match initial values")
	}
	c2s[0] = 9
	s2c[0] = 9
	gotC2S2, gotS2C2 := sm.CurrentKeys()
	if gotC2S2[0] == 9 || gotS2C2[0] == 9 {
		t.Fatalf("expected internal keys to be independent copies")
	}
}

func TestStartRekey_StagesEpochAndDoesNotSwitchSendUntilAck(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 10}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 10 {
		t.Fatalf("expected epoch=10, got %d", epoch)
	}
	if sm.ReadyForRekey() {
		t.Fatal("expected staged epoch to block another rekey")
	}
	if sm.state.staged.epoch != 10 {
		t.Fatalf("expected staged epoch=10, got %+v", sm.state.staged)
	}
	if sm.state.phase != phaseStaged {
		t.Fatalf("expected staged phase, got %v", sm.state.phase)
	}

	// Must not switch send epoch until confirmed.
	if sm.state.current.epoch != 0 {
		t.Fatalf("expected sendEpoch still 0, got %d", sm.state.current.epoch)
	}

	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected PromoteSendEpoch not called yet, got %v", setCalls)
	}
}

func TestStartRekey_RejectsInvalidKeySize(t *testing.T) {
	for _, size := range []int{chacha20poly1305.KeySize - 1, chacha20poly1305.KeySize + 1} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			mock := &StateMachineEpochManagerMock{installedEpoch: 1}
			sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

			if _, err := sm.StartRekey(make([]byte, size), stateMachineTestKey("new-s2c")); err == nil {
				t.Fatal("expected invalid key size to be rejected")
			}

			installCalls, _, _, _ := mock.Snapshot()
			if installCalls != 0 {
				t.Fatalf("expected StageEpoch not to be called, got %d calls", installCalls)
			}
			if !sm.ReadyForRekey() || sm.state.phase != phaseIdle {
				t.Fatalf("expected idle machine, got %+v", sm.state)
			}
		})
	}
}

func TestStartRekey_ReusesStagedStorageWithoutAllocations(t *testing.T) {
	manager := &noAllocEpochManager{}
	keySize := chacha20poly1305.KeySize
	sm := NewStateMachine(manager, make([]byte, keySize), make([]byte, keySize))
	c2s := make([]byte, len(sm.state.staged.c2s))
	s2c := make([]byte, len(sm.state.staged.s2c))

	var (
		epoch  uint16
		runErr error
	)
	allocs := testing.AllocsPerRun(100, func() {
		epoch, runErr = sm.StartRekey(c2s, s2c)
		if runErr == nil {
			sm.ActivateSendEpoch(epoch)
			sm.ObservePeerEpoch(epoch)
		}
	})
	if runErr != nil {
		t.Fatalf("StartRekey: %v", runErr)
	}
	if allocs != 0 {
		t.Fatalf("expected staged storage to be reused, got %.2f allocations per run", allocs)
	}
}

func TestActivateSendEpoch_PromotesKeysAndWaitsForPeerObservation(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 7}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}

	sm.ActivateSendEpoch(epoch)

	if sm.state.current.epoch != epoch {
		t.Fatalf("expected sendEpoch=%d, got %d", epoch, sm.state.current.epoch)
	}
	if sm.state.phase != phaseRetiring {
		t.Fatalf("expected retiring phase, got %v", sm.state.phase)
	}
	if sm.ReadyForRekey() {
		t.Fatal("expected previous epoch retirement to block another rekey")
	}

	gotC2S, gotS2C := sm.CurrentKeys()
	if !reflect.DeepEqual(gotC2S, stateMachineTestKey("new-c2s")) ||
		!reflect.DeepEqual(gotS2C, stateMachineTestKey("new-s2c")) {
		t.Fatalf("expected promoted keys new-c2s/new-s2c, got %q/%q", gotC2S, gotS2C)
	}

	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 1 || setCalls[0] != epoch {
		t.Fatalf("expected PromoteSendEpoch(%d) once, got %v", epoch, setCalls)
	}

	sm.ObservePeerEpoch(epoch)
	if !sm.ReadyForRekey() {
		t.Fatal("expected machine to become ready after previous epoch retirement")
	}
}

func TestActivateSendEpoch_DoesNotActivateIfEpochNotConfirmed(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 10}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey unexpected error: %v", err)
	}
	if epoch != 10 {
		t.Fatalf("expected epoch=10, got %d", epoch)
	}

	// Confirm a smaller epoch than staged; should not activate.
	sm.ActivateSendEpoch(9)

	if sm.ReadyForRekey() {
		t.Fatal("expected staged epoch to remain")
	}
	if sm.state.current.epoch != 0 {
		t.Fatalf("expected sendEpoch still 0, got %d", sm.state.current.epoch)
	}

	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no PromoteSendEpoch calls, got %v", setCalls)
	}
}

func TestActivateSendEpoch_WaitsForRekeyAndActivatesStaged(t *testing.T) {
	installEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineEpochManagerMock{
		installedEpoch: 42,
		installEntered: installEntered,
		installBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	startDone := make(chan struct{})
	var startEpoch uint16
	var startErr error
	go func() {
		defer close(startDone)
		startEpoch, startErr = sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	}()

	// Wait until StageEpoch is entered while StartRekey holds the FSM mutex.
	select {
	case <-installEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	activateDone := make(chan struct{})
	go func() {
		defer close(activateDone)
		sm.ActivateSendEpoch(42)
	}()

	// Let StageEpoch finish. ActivateSendEpoch then acquires the FSM mutex and activates staged.
	close(rekeyUnblock)

	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartRekey to finish")
	}
	select {
	case <-activateDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for ActivateSendEpoch to finish")
	}

	if startErr != nil {
		t.Fatalf("StartRekey unexpected error: %v", startErr)
	}
	if startEpoch != 42 {
		t.Fatalf("expected epoch=42, got %d", startEpoch)
	}
	if sm.state.current.epoch != 42 {
		t.Fatalf("expected sendEpoch=42, got %d", sm.state.current.epoch)
	}
	if sm.state.phase != phaseRetiring {
		t.Fatalf("expected retiring phase, got %v", sm.state.phase)
	}
	gotC2S, gotS2C := sm.CurrentKeys()
	if !reflect.DeepEqual(gotC2S, stateMachineTestKey("new-c2s")) ||
		!reflect.DeepEqual(gotS2C, stateMachineTestKey("new-s2c")) {
		t.Fatalf("expected promoted keys new-c2s/new-s2c, got %q/%q", gotC2S, gotS2C)
	}

	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 1 || setCalls[0] != 42 {
		t.Fatalf("expected PromoteSendEpoch(42) once, got %v", setCalls)
	}
}

func TestStartRekey_ConcurrentCallWaitsThenFailsWhenStaged(t *testing.T) {
	installEntered := make(chan struct{})
	rekeyUnblock := make(chan struct{})

	mock := &StateMachineEpochManagerMock{
		installedEpoch: 1,
		installEntered: installEntered,
		installBlock:   rekeyUnblock,
	}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	firstDone := make(chan error, 1)
	go func() {
		_, err := sm.StartRekey(stateMachineTestKey("k1"), stateMachineTestKey("k2"))
		firstDone <- err
	}()

	select {
	case <-installEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Rekey to be entered")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := sm.StartRekey(stateMachineTestKey("k3"), stateMachineTestKey("k4"))
		secondDone <- err
	}()

	close(rekeyUnblock)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first StartRekey failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for first StartRekey to finish")
	}
	select {
	case err := <-secondDone:
		if err == nil {
			t.Fatal("expected concurrent StartRekey to fail after observing staged epoch")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for second StartRekey to finish")
	}

	if sm.ReadyForRekey() || sm.state.phase != phaseStaged {
		t.Fatalf("expected first epoch to remain staged, got %+v", sm.state)
	}
}

func TestStartRekey_CryptoError_LeavesMachineReady_NoCleanupEpoch(t *testing.T) {
	sentinelErr := errors.New("crypto failure")
	mock := &StateMachineEpochManagerMock{installErr: sentinelErr}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	_, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected crypto error, got %v", err)
	}
	if !sm.ReadyForRekey() {
		t.Fatal("expected machine to remain ready after crypto error")
	}

	installCalls, _, _, _ := mock.Snapshot()
	if installCalls != 1 {
		t.Fatalf("expected StageEpoch called once, got %d", installCalls)
	}
}

func TestStartRekey_CopiesInputSlices_NoExternalMutationLeak(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 3}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	c2s := make([]byte, chacha20poly1305.KeySize)
	s2c := make([]byte, chacha20poly1305.KeySize)
	c2s[0] = 1
	s2c[0] = 4

	epoch, err := sm.StartRekey(c2s, s2c)
	if err != nil {
		t.Fatalf("StartRekey failed: %v", err)
	}

	c2s[0] = 9
	s2c[0] = 9

	sm.ActivateSendEpoch(epoch)
	gotC2S, gotS2C := sm.CurrentKeys()
	if gotC2S[0] != 1 {
		t.Fatalf("expected promoted C2S key to retain owned value, got %v", gotC2S)
	}
	if gotS2C[0] != 4 {
		t.Fatalf("expected promoted S2C key to retain owned value, got %v", gotS2C)
	}
}

func TestObservePeerEpoch_TracksMaxAuthenticatedEpoch(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 2}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	sm.ObservePeerEpoch(10)
	sm.ObservePeerEpoch(7)
	sm.ObservePeerEpoch(11)

	if sm.state.maxObservedPeerEpoch != 11 {
		t.Fatalf("expected maxObservedPeerEpoch=11, got %d", sm.state.maxObservedPeerEpoch)
	}
}

func TestObservePeerEpoch_DoesNotActivateStagedEpoch(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 4}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))
	if _, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c")); err != nil {
		t.Fatalf("StartRekey: %v", err)
	}

	sm.ObservePeerEpoch(4)

	if sm.state.current.epoch != 0 || sm.state.phase != phaseStaged || sm.state.staged.epoch != 4 {
		t.Fatalf("observation must not activate send: send=%d staged=%+v", sm.state.current.epoch, sm.state.staged)
	}
	if sm.ReadyForRekey() {
		t.Fatal("expected staged epoch to block another rekey")
	}
	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no send activation, got %v", setCalls)
	}
}

func TestEpochRetiresAfterSendActivationAndPeerObservation(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 4}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))
	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey: %v", err)
	}

	sm.ActivateSendEpoch(epoch)
	if calls := mock.RetireCalls(); calls != 0 {
		t.Fatalf("expected no retirement before peer observation, got %d calls", calls)
	}

	sm.ObservePeerEpoch(epoch)
	if calls := mock.RetireCalls(); calls != 1 {
		t.Fatalf("expected one RetirePreviousEpoch call, got %d", calls)
	}
	if sm.state.phase != phaseIdle {
		t.Fatalf("expected completed retirement to be cleared, got %+v", sm.state)
	}
}

func TestStartRekey_RefusesUntilPreviousEpochRetired(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 1}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))

	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey: %v", err)
	}
	sm.ActivateSendEpoch(epoch)

	mock.mu.Lock()
	mock.installedEpoch = 2
	mock.mu.Unlock()

	if _, err := sm.StartRekey(stateMachineTestKey("next-c2s"), stateMachineTestKey("next-s2c")); err == nil {
		t.Fatal("expected rekey to be refused while previous epoch is retained")
	}
	installCalls, _, _, _ := mock.Snapshot()
	if installCalls != 1 {
		t.Fatalf("expected refused rekey not to stage another epoch, got %d stage calls", installCalls)
	}

	sm.ObservePeerEpoch(epoch)
	if !sm.ReadyForRekey() {
		t.Fatal("expected machine to become ready after previous epoch retirement")
	}

	nextEpoch, err := sm.StartRekey(stateMachineTestKey("next-c2s"), stateMachineTestKey("next-s2c"))
	if err != nil {
		t.Fatalf("StartRekey after retirement: %v", err)
	}
	if nextEpoch != 2 {
		t.Fatalf("expected epoch 2, got %d", nextEpoch)
	}
}

func TestEpochRetiresWhenPeerWasObservedBeforeSendActivation(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 4}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))
	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey: %v", err)
	}

	sm.ObservePeerEpoch(epoch)
	sm.ActivateSendEpoch(epoch)

	if calls := mock.RetireCalls(); calls != 1 {
		t.Fatalf("expected one RetirePreviousEpoch call, got %d", calls)
	}
}

func TestEpochRetirementRetriesAfterCryptoRefusal(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 4, retireEpochFails: true}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))
	epoch, err := sm.StartRekey(stateMachineTestKey("new-c2s"), stateMachineTestKey("new-s2c"))
	if err != nil {
		t.Fatalf("StartRekey: %v", err)
	}
	sm.ActivateSendEpoch(epoch)
	sm.ObservePeerEpoch(epoch)
	if sm.state.phase != phaseRetiring {
		t.Fatal("expected refused retirement to remain pending")
	}

	mock.mu.Lock()
	mock.retireEpochFails = false
	mock.mu.Unlock()
	sm.ObservePeerEpoch(epoch)

	if sm.state.phase != phaseIdle {
		t.Fatalf("expected retirement to clear after retry, got %+v", sm.state)
	}
	if calls := mock.RetireCalls(); calls != 2 {
		t.Fatalf("expected two retirement attempts, got %d", calls)
	}
}

func TestStartRekey_PreservesCanonicalKeys(t *testing.T) {
	mock := &StateMachineEpochManagerMock{installedEpoch: 3}
	sm := NewStateMachine(mock, []byte("old-c2s"), []byte("old-s2c"))
	newC2S := stateMachineTestKey("new-c2s")
	newS2C := stateMachineTestKey("new-s2c")

	epoch, err := sm.StartRekey(newC2S, newS2C)
	if err != nil {
		t.Fatalf("StartRekey failed: %v", err)
	}
	_, stagedC2S, stagedS2C, _ := mock.Snapshot()
	if len(stagedC2S) != 1 || !reflect.DeepEqual(stagedC2S[0], newC2S) {
		t.Fatalf("expected staged C2S key, got %v", stagedC2S)
	}
	if len(stagedS2C) != 1 || !reflect.DeepEqual(stagedS2C[0], newS2C) {
		t.Fatalf("expected staged S2C key, got %v", stagedS2C)
	}

	sm.ActivateSendEpoch(epoch)

	gotC2S, gotS2C := sm.CurrentKeys()
	if !reflect.DeepEqual(gotC2S, newC2S) {
		t.Fatalf("expected canonical C2S key, got %q", gotC2S)
	}
	if !reflect.DeepEqual(gotS2C, newS2C) {
		t.Fatalf("expected canonical S2C key, got %q", gotS2C)
	}
}

func TestActivateSendEpoch_NoOpWithoutStagedEpoch(t *testing.T) {
	mock := &StateMachineEpochManagerMock{}
	sm := NewStateMachine(mock, []byte("c2s"), []byte("s2c"))

	sm.ActivateSendEpoch(5)

	if !sm.ReadyForRekey() {
		t.Fatal("expected machine without staged epoch to remain ready")
	}
	_, _, _, setCalls := mock.Snapshot()
	if len(setCalls) != 0 {
		t.Fatalf("expected no PromoteSendEpoch calls, got %v", setCalls)
	}
}
