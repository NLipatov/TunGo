package bubble_tea

import tea "charm.land/bubbletea/v2"

type programRunner interface {
	Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error)
}

type bubbleProgramRunner struct{}

func newProgramRunner() programRunner {
	return &bubbleProgramRunner{}
}

func (r *bubbleProgramRunner) Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
	p := tea.NewProgram(model, opts...)
	return p.Run()
}
