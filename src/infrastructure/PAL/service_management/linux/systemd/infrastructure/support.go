package infrastructure

func Supported(h Hooks, runtimeDir string) bool {
	if _, err := h.Stat(runtimeDir); err != nil {
		return false
	}
	if _, err := h.LookPath("systemctl"); err != nil {
		return false
	}
	return true
}
