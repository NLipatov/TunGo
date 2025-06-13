package tui

type (
	TextInput interface {
		Value() (string, error)
	}
	TextInputFactory interface {
		NewTextInput(placeholder string) (TextInput, error)
	}
)
