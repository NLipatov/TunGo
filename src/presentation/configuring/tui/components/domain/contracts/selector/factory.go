package selector

import "tungo/presentation/configuring/tui/components/domain/value_objects"

type Factory interface {
	NewTuiSelector(
		placeholder string,
		options []string,
		foregroundColor value_objects.Color,
		backgroundColor value_objects.Color,
	) (Selector, error)
}
