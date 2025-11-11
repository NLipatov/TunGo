package text_area

type TextAreaFactory interface {
	NewTextArea(placeholder string) (TextArea, error)
}
