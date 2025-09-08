package mtu

import "fmt"

type Producer struct {
	bestMTU          int
	maximum, minimum int
}

func NewProducer(minimum, maximum int) *Producer {
	if minimum > maximum {
		minimum, maximum = maximum, minimum // normalize instead of surprising the caller
	}
	return &Producer{
		maximum: maximum,
		minimum: minimum,
	}
}

func (p *Producer) MTU() (int, error) {
	// prevent stale value leak across runs
	p.bestMTU = -1
	// discover new MTU value
	p.discoverMTU(p.minimum, p.maximum)
	if p.bestMTU == -1 {
		return 0, fmt.Errorf("failed to discover best MTU")
	}
	return p.bestMTU, nil
}

func (p *Producer) discoverMTU(minimum, maximum int) {
	if minimum > maximum {
		return
	}
	current := minimum + (maximum-minimum)/2
	if err := p.tryMtu(current); err != nil {
		// current fails: search lower
		p.discoverMTU(minimum, current-1)
	} else {
		// current succeeded: store and search higher
		p.bestMTU = current
		p.discoverMTU(current+1, maximum)
	}
}

func (p *Producer) tryMtu(mtu int) error {
	panic("not implemented")
}
