package rekey

type State int

const (
	StateStable State = iota
	// StateRekeying means we started Rekey() but have not yet applied keys.
	StateRekeying
	// StatePending means new keys are installed for receive; send switch awaits confirmation.
	StatePending
)
