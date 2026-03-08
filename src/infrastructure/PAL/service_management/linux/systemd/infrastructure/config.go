package infrastructure

type Config struct {
	RuntimeDir string
	UnitPath   string
	UnitName   string
	BinaryPath string
}

func DefaultConfig() Config {
	return Config{
		RuntimeDir: "/run/systemd/system",
		UnitPath:   "/etc/systemd/system/tungo.service",
		UnitName:   "tungo.service",
		BinaryPath: "/usr/local/bin/tungo",
	}
}
