# Tunnel E2E

The [workflow](../../workflows/main.yml) builds TunGo and prepares two machines:
a native Linux/macOS/Windows client on the GitHub runner and a Linux server VM.
[runner/run.sh](runner/run.sh) owns QEMU, networking and temporary SSH access.
The [harness](harness/e2e.go) owns the complete TunGo scenario:
generate configurations on the server, start it, run the client over SSH,
check traffic, then stop the server and verify cleanup.

```text
                       harness run (on the runner)
                         /                   \
                       SSH              SSH localhost
                        |                     |
                    Linux VM             GitHub runner
                  TunGo server <=== VPN === TunGo client
                        |                harness client:
                      nginx              ping / traceroute / curl
```

The same executable has two commands:

- `tungo-e2e run UDP <workdir>` orchestrates both sides over SSH.
- `tungo-e2e client <workdir>` runs client checks on the client machine.
  It receives the configuration and expected payload checksum through SSH stdin.

The workflow runs `runner/run.sh UDP arm64 "$RUNNER_TEMP"` and tests 36 combinations:
six native client OS/architecture pairs × two Linux server architectures × UDP/TCP/WS.
The harness does not create VMs or select architectures. Both TunGo binaries are
built from the current checkout. OS-specific SSH preparation lives in [runner/](runner/).

Each of two connections checks IPv4 and IPv6: ping, the server TUN as the first
traceroute hop, both halves of the default route, a 4 MiB download with SHA-256,
and NAT source. The target is healthy but unreachable before and after connection.
Forwarding starts disabled and must be enabled by TunGo; the target has no return
route, so replies require NAT. Client TUN addresses and route selection must be
restored; server interfaces, routes and firewall are compared after shutdown.
The test does not compare the client's entire firewall/routing table or test DNS.

The work directory contains `server.json` (`SSH`, `VPN`) and `client.json`
(`SSH`, `User`, `Command`), native binaries and diagnostics in `tungo-e2e-logs/`.
Only connection details and the OS-specific harness launch command come from setup;
TunGo configurations are created by the harness. SSH uses temporary keys in
`tungo-ssh/`; both host keys are pinned locally. The runner script stops SSH/QEMU
and removes its keys on exit, including failure. Key files are not uploaded.

Build with `go build`. The `*_unit_test.go` files test the harness itself with
`go test`; Windows CI also checks graceful client shutdown with and without an
inherited console. E2E requires disposable GitHub Actions machines. The scenario
has an 8-minute deadline, the client part 5 minutes, each connection 90 seconds.
