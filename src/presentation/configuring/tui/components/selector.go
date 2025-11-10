package components

type (
	Selector interface {
		SelectOne() (string, error)
	}

	SelectorFactory interface {
		NewTuiSelector(
			placeholder string,
			options []string,
			foregroundColor Color,
			backgroundColor Color,
		) (Selector, error)
	}

	Colorizer interface {
		ColorizeString(
			s string,
			foreground, background Color,
		) string
	}
)
