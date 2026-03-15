package infrastructure

import "fmt"

func UnitFileContent(binaryPath string, modeArg string) string {
	return fmt.Sprintf(`[Unit]
Description=TunGo VPN Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s %s
User=root
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, binaryPath, modeArg)
}
