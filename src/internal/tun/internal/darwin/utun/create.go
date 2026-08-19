//go:build darwin

package utun

type mtuSetter interface {
	SetMTU(ifName string, mtu int) error
}

func Create(ifConfig mtuSetter, mtu int) (UTUN, error) {
	u, err := newRawUTUN()
	if err != nil {
		return nil, err
	}
	ifName := u.name

	if setMTUErr := ifConfig.SetMTU(ifName, mtu); setMTUErr != nil {
		_ = u.Close()
		return nil, setMTUErr
	}

	return u, nil
}
