package bubble_tea

import tea "github.com/charmbracelet/bubbletea"

type TeaRunner interface {
	Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error)
}

type defaultTeaRunner struct{}

func (r *defaultTeaRunner) Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
	p := tea.NewProgram(model, opts...)
	return p.Run()
}
