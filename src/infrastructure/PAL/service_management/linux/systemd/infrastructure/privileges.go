package infrastructure

import "errors"

func RequireAdminPrivileges(h Hooks) error {
	if h.Geteuid == nil {
		return errors.New("admin privileges are required to manage tungo systemd service")
	}
	if h.Geteuid() == 0 {
		return nil
	}
	return errors.New("admin privileges are required to manage tungo systemd service")
}
