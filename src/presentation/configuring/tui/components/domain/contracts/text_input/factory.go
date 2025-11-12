package text_input

type TextInputFactory interface {
	NewTextInput(placeholder string) (TextInput, error)
}
