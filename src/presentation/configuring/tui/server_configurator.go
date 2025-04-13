package tui

type serverConfigurator struct{}

func newServerConfigurator() *serverConfigurator {
	return &serverConfigurator{}
}

func (s *serverConfigurator) Configure() error {
	return nil
}
