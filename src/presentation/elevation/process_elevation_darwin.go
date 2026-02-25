package elevation

import "os"

func IsElevated() bool {
	return os.Geteuid() == 0
}

func Hint() string {
	return "Please restart with 'sudo'."
}
