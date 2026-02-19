package uifactory

import (
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_area"
	"tungo/presentation/ui/tui/internal/ui/contracts/text_input"
)

// Bundle keeps UI component factories implementation-agnostic for callers.
type Bundle struct {
	SelectorFactory  selector.Factory
	TextInputFactory text_input.TextInputFactory
	TextAreaFactory  text_area.TextAreaFactory
}

func NewDefaultBundle() Bundle {
	return Bundle{
		SelectorFactory:  bubbleTea.NewSelectorAdapter(),
		TextInputFactory: bubbleTea.NewTextInputAdapter(),
		TextAreaFactory:  bubbleTea.NewTextAreaAdapter(),
	}
}
