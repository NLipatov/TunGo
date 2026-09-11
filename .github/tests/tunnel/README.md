# Linux tunnel E2E

Run from a Linux host with Docker access and `/dev/net/tun`:

```bash
bash .github/tests/tunnel/run.sh UDP
bash .github/tests/tunnel/run.sh TCP
bash .github/tests/tunnel/run.sh WS
```

Each invocation builds the current checkout and test tools using the Go version
from `src/go.mod`. The test Dockerfile builds for the Docker host's native
architecture, without emulation or a hardcoded `GOARCH`. `EXPECTED_ARCH` can assert
that the host matches the intended architecture. It creates two internal bridges:

| Container | Transport network | Target network |
| --- | --- | --- |
| TunGo client | Yes | No |
| TunGo server | Yes | Yes |
| HTTP target | No | Yes |

Client and server run as root with `NET_ADMIN` and `/dev/net/tun`. The server's
forwarding sysctl is enabled at container creation, and its default route points
at the target network so TunGo selects the correct NAT interface. Its FORWARD
policy starts at DROP: successful traffic must use TunGo's forwarding rules.

The client has no default route before startup. The target is reachable only
after TunGo installs its split default routes. The test checks that the target
is inaccessible before and after each client connection, while it remains alive
and reachable from the server. No public endpoint is used for traffic checks.

For each transport, the script generates a fresh enrolled client via `tungo s gen`
and checks a real TUN-address ping, a 4 MiB HTTP download with SHA-256 validation,
and the source IP seen by the target to verify MASQUERADE. It stops and starts the
client once more to test a fresh connection after teardown. TunGo is stopped with
SIGTERM while the containers stay alive, allowing comparison of routes and
firewall rules against their baselines and detection of leftover TUN interfaces.

This is IPv4 Linux coverage. It does not test DNS configuration, IPv6, WSS/TLS,
packet-loss recovery, other kernels, or macOS/Windows. The plain Docker resolver
is unsupported by TunGo's DNS configurator, so its DNS warning is expected here.
The test deliberately does not claim that DNS configuration works.

The main workflow runs all three transports on both `ubuntu-24.04` (amd64) and
`ubuntu-24.04-arm` (arm64), for six independent native runs. Client and server
share the runner's architecture. The workflow waits for all six before creating
a version tag. Changes to this directory or the main workflow
also trigger CI. Logs and network snapshots are uploaded even on failures;
generated key material is never included. Locally, `ARTIFACT_DIR` selects the
output directory, otherwise a temporary directory is created and printed.
Containers, networks and temporary image tags are removed on exit.

The eventual full matrix is six native clients (Linux/macOS/Windows on amd64 and
arm64) against two Linux server architectures over three transports: 36 cases.
That requires a network-accessible server fixture for the macOS/Windows jobs and
mixed-architecture pairs. These six tests are the first stage, not that full
compatibility matrix.
