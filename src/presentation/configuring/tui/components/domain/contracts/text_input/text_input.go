package text_input

type TextInput interface {
	Value() (string, error)
}
