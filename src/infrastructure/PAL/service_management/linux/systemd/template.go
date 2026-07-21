package systemd

import (
	"fmt"
	"strconv"
	"strings"
)

func UnitFileContent(binaryPath string, args []string) string {
	execStart := strconv.Quote(binaryPath)
	if len(args) > 0 {
		execStart += " " + strings.Join(args, " ")
	}
	return fmt.Sprintf(`[Unit]
Description=TunGo VPN Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
User=root
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, execStart)
}
