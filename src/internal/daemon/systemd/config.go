package systemd

type Config struct {
	RuntimeDir string
	UnitDir    string
	BinaryPath string
}

func DefaultConfig() Config {
	return Config{
		RuntimeDir: "/run/systemd/system",
		UnitDir:    "/etc/systemd/system",
		BinaryPath: "/usr/local/bin/tungo",
	}
}
