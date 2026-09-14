# Tunnel E2E

Run the IPv4/IPv6 [container tests](containers/) from the repository root on a
Linux Docker host with Docker access and `/dev/net/tun`. Each command builds the
current checkout.

```bash
bash .github/tests/tunnel/containers/run.sh UDP
bash .github/tests/tunnel/containers/run.sh TCP
bash .github/tests/tunnel/containers/run.sh WS
```

`ARTIFACT_DIR` selects the diagnostics directory; otherwise the script creates
a temporary directory and prints its location.

Each run checks both IP families over two connections: TUN routes and ping,
payload checksum, NAT, isolation without a tunnel, and network cleanup.

The [compatibility tests](compatibility/) run native Linux/macOS/Windows clients
against Linux servers under QEMU TCG, with IPv4 and IPv6 tunnel traffic. CI builds
the server VM from [server-vm](compatibility/server-vm/).
Both suites use an IPv4 VPN endpoint to carry traffic from both IP families.

The Go [harness](harness/) has three modes: `run` (runner), `agent`, and `target`.

```text
Traffic: runner -> TunGo client == VPN ==> TunGo server -> HTTP target
Control: runner -> agent -> TunGo server
```

The runner and TunGo client run on the native client host; the agent, server
and target run inside the Linux VM.

The runner boots the VM, starts the native client, and checks traffic, checksums,
NAT and cleanup over two connections. The agent controls the server and reports
target health, process status and diagnostics. The target serves test data.

`tungo-e2e run` is for disposable GitHub Actions runners only: it requires
administrative privileges and changes host routes. See the
[main workflow](../../workflows/main.yml) for VM setup and invocation.
