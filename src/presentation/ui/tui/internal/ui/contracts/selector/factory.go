package selector

import "tungo/presentation/ui/tui/internal/ui/value_objects"

type Factory interface {
	NewTuiSelector(
		placeholder string,
		options []string,
		foregroundColor value_objects.Color,
		backgroundColor value_objects.Color,
	) (Selector, error)
}
