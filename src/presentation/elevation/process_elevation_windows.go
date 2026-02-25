package elevation

import "golang.org/x/sys/windows"

func IsElevated() bool {
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

func Hint() string {
	return "Please restart the application as Administrator (right-click -> 'Run as Administrator')."
}
