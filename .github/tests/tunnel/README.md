# Tunnel E2E harness

The harness starts TunGo on a prepared server and client, then checks traffic:

```text
TunGo client -> TunGo server -> HTTP target
```

The HTTP target is on an isolated network behind the server.
The client can reach it only through the tunnel.

## Requirements

### Client requirements

Linux/macOS/Windows with SSH access and root or administrator privileges.
The client needs TUN support, `curl`, `ping` and `traceroute` (`tracert` on Windows;
`ping6`/`traceroute6` for IPv6 on macOS).

Install the client harness and `tungo-client` (`tungo-client.exe` on Windows) on
the client; the TunGo binary must be in `<client-workdir>`.

### Server requirements

Linux with root SSH access. The server needs `tungo`, `network-state`, `curl`,
`sha256sum` and the network and HTTP target defined in the [server fixture](server-vm/).

## Build harness

Run from the `harness/` directory.

### Linux/macOS

```sh
go build -o tungo-e2e .
```

### Windows

```powershell
go build -o tungo-e2e.exe .
```

## Usage

Prepare these inputs in `<workdir>`:

| File | Contents |
| --- | --- |
| `server.json` | `SSH`: server `host:port`; `VPN`: server transport IP address |
| `client.json` | `SSH`: client `host:port`; `User`: SSH user; `Command`: command that launches `tungo-e2e client <client-workdir>` |
| `tungo-ssh/identity` | SSH private key authorized on both hosts |
| `tungo-ssh/server.pub`, `tungo-ssh/client.pub` | Pinned SSH host public keys |

Run one protocol at a time:

```text
tungo-e2e run <UDP|TCP|WS> <workdir>
```

`run` generates TunGo configurations on the server, starts it, and launches
`client` over SSH. It supplies JSON fields `Config` and `Checksum` through stdin;
`client` installs the configuration, starts TunGo and checks traffic.
After the client checks, `run` stops the server and verifies cleanup.

The harness creates server and client configurations at TunGo's standard paths.
It fails if either configuration file already exists, to avoid overwriting it.

Each of two connections checks IPv4 and IPv6: ping, the server TUN as the first
traceroute hop, both halves of the default route, a 4 MiB download with SHA-256,
and NAT source. The target is healthy but unreachable before and after connection.
Forwarding starts disabled and must be enabled by TunGo; the target has no return
route, so replies require NAT. The client TUN interface is identified while connected
and must disappear after shutdown, along with its addresses; route selection must be
restored. Server interfaces, routes and firewall are compared after shutdown.
The test does not compare the client's entire firewall/routing table or test DNS.

`run` prints `PASS` on success; failure exits with code 1. Diagnostics are written
to `tungo-e2e-logs/` in each command's work directory. The scenario has an 8-minute deadline,
the client part 5 minutes, each connection 90 seconds.

Run `go test ./...` from `harness/` to test the harness code itself.
