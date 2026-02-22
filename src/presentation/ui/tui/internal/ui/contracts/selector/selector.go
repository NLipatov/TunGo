package selector

type Selector interface {
	SelectOne() (string, error)
}
