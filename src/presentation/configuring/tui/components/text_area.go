package components

type (
	TextArea interface {
		Value() (string, error)
	}

	TextAreaFactory interface {
		NewTextArea(placeholder string) (TextArea, error)
	}
)
