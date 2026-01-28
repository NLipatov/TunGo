package rekey

type State int

const (
	StateStable State = iota
	// StateInstalling means we started Rekey() but have not yet applied keys.
	StateInstalling
	// StatePending means new keys are installed for receive; send switch awaits confirmation.
	StatePending
)
