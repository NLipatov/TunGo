package elevation

import "golang.org/x/sys/windows"

// ProcessElevationImpl implements ProcessElevation on Windows
type ProcessElevationImpl struct{}

func NewProcessElevation() ProcessElevation {
	return &ProcessElevationImpl{}
}

func (p *ProcessElevationImpl) IsElevated() bool {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}
