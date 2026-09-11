# Tunnel E2E

CI runs 36 compatibility cases plus six fully native Linux container tests.
All binaries are built from the checked-out commit. Tests require real TUN
devices and administrative privileges; cross-compilation alone does not pass.

## Native Linux container tests

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

## Full compatibility matrix

`tunnel-compatibility` expands every row below against both Linux server
architectures and all three transports, UDP/TCP/WS: **6 x 2 x 3 = 36 cases**.

| Native client | Architecture | Runner |
| --- | --- | --- |
| Linux | amd64 | ubuntu-24.04 |
| Linux | arm64 | ubuntu-24.04-arm |
| macOS | amd64 | macos-15-intel |
| macOS | arm64 | macos-15 |
| Windows | amd64 | windows-2025 |
| Windows | arm64 | windows-11-arm |

Two native Linux build jobs produce a kernel and an Alpine initramfs containing
the server binary. Each compatibility job boots the selected architecture using
QEMU TCG, then builds and runs the native client directly on its runner OS.
Both the native client build architecture and running guest kernel architecture
are asserted. **Servers in this matrix are CPU-emulated**, including pairs of
matching architectures. The original six container jobs retain native server
execution. This is functional compatibility coverage, not a performance test.

The guest contains the server and an HTTP target in a separate network namespace:

| Address | Purpose |
| --- | --- |
| 192.0.2.15 | Guest transport and fixture controller |
| 198.18.0.1 | Server backend interface / expected NAT source |
| 198.18.0.2 | Isolated HTTP target |
| 198.19.0.1 | Server TUN address |

macOS uses QEMU's host-only vmnet network because TunGo rejects loopback server
addresses on macOS. Linux/Windows forward only the controller and VPN listener
ports through QEMU's restricted user network, bound to host loopback addresses.
The HTTP target is never port-forwarded. Guest firewall rules also prohibit
direct forwarding from the transport interface into the target network.

Before starting the client, the harness installs more-specific routes for the
complement of `198.18.0.0/15` through the original runner gateway. This preserves
GitHub runner communication while leaving **both test networks exclusively to
TunGo's routes**. It asserts the split default routes and target's TUN interface
after connection. No test-target routes are injected by the harness. These jobs
do not test tunneling the runner's entire public-internet traffic.

Each case verifies no connectivity before startup, live target health, real TUN
ping, a 4 MiB download with SHA-256, target-observed NAT source, and two complete
client start/stop cycles. After each stop it compares routes and interfaces to
the baseline; Linux also compares firewall rules. Windows uses Ctrl+Break to
exercise graceful shutdown rather than TerminateProcess. The server receives
SIGTERM and must restore routes/firewall and remove its TUN before the VM exits.

`vm/run.py` is restricted to disposable Actions runners because it changes host
routing. Its finally block stops processes, removes the generated configuration,
and removes fixture routes. Diagnostic artifacts contain logs and network
snapshots. VM images are built before keys are generated and contain no client
or server key material. The fixture needs no repository secrets or external
server. Version tagging waits for both test suites.

Neither suite asserts DNS configuration/restoration, IPv6, WSS/TLS, automatic
network-loss recovery, or throughput. A second client launch is a fresh
connection test, not a simulated network outage.
