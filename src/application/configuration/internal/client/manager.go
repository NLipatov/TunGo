package client

type Manager struct {
	resolver Resolver
}

func NewManager() *Manager {
	return &Manager{
		resolver: NewDefaultResolver(),
	}
}

func (m *Manager) Configuration() (*Configuration, error) {
	path, err := m.resolver.Resolve()
	if err != nil {
		return nil, err
	}
	return read(path)
}
