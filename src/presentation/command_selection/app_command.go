package command_selection

import "tungo/domain/command"

// AppCommand resolves the application's command from process arguments.
type AppCommand interface {
	Command() (command.Command, error)
}
