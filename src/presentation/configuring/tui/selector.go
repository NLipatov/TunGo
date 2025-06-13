package tui

type (
	Selector interface {
		SelectOne() (string, error)
	}

	SelectorFactory interface {
		NewTuiSelector(placeholder string, options []string) (Selector, error)
	}
)
