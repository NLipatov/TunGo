package rekey

type fsmEffect interface {
	apply(crypto Rekeyer)
}

type effectSetSendEpoch struct{ epoch uint16 }

func (e effectSetSendEpoch) apply(crypto Rekeyer) { crypto.SetSendEpoch(e.epoch) }

type effectRemoveEpoch struct{ epoch uint16 }

func (e effectRemoveEpoch) apply(crypto Rekeyer) { _ = crypto.RemoveEpoch(e.epoch) }
