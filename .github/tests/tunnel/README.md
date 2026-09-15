# Tunnel E2E harness

The harness uses prepared server and client machines:

1. `coordinator` connects over SSH, generates TunGo configurations and starts the server.
2. It launches the same harness in `client` mode over SSH and sends the configuration
   through stdin. The client installs it and runs two connect/check/disconnect cycles.
3. The coordinator stops the server. Both sides are checked for network cleanup.

```text
Control:
tungo-e2e coordinator
  |-- SSH --> server: tungo s
  \-- SSH --> client: tungo-e2e client
                       |-- tungo c
                       \-- traffic and cleanup checks

Traffic:
TunGo client -> TunGo server -> HTTP target
```

The HTTP target is on an isolated network behind the server, reachable by the client
only through the tunnel. It must be healthy but unreachable before and after connection.

Each connection checks IPv4 and IPv6: ping, server and target traceroute hops, both
halves of the default route, a 4 MiB download with SHA-256, and NAT source.
Forwarding starts disabled; the target has no return route, requiring TunGo's forwarding and NAT.
After shutdown, the client TUN and its addresses must disappear and route selection
must match the baseline; server interfaces, routes and firewall must match too.
DNS and the client's entire firewall/routing table are not checked.

## Requirements

### Client requirements

Linux/macOS/Windows, SSH access, root/administrator privileges, TUN support,
`curl`, `ping` and `traceroute` (`tracert` on Windows; `ping6`/`traceroute6` on macOS).
Install the harness on the client and put `tungo-client` (`tungo-client.exe` on Windows)
in `<client-workdir>`.

### Server requirements

Linux, root SSH access, `tungo`, `network-state`, `curl`, `sha256sum`, and the
network and HTTP target defined in the [server fixture](server-vm/).

## Build harness

From `harness/`, build for each machine that runs the harness.

### Linux/macOS

`go build -o tungo-e2e .`

### Windows

`go build -o tungo-e2e.exe .`

## Usage

Prepare these inputs in `<workdir>`:

| File | Contents |
| --- | --- |
| `server.json` | `SSH`: server `host:port`; `VPN`: server transport IP address |
| `client.json` | `SSH`: client `host:port`; `User`: SSH user; `Command`: `tungo-e2e client <client-workdir>` with the installed harness path |
| `tungo-ssh/identity` | SSH private key authorized on both hosts |
| `tungo-ssh/server.pub`, `tungo-ssh/client.pub` | Pinned SSH host public keys |

Run one protocol at a time:

```text
tungo-e2e coordinator <UDP|TCP|WS> <workdir>
```

Existing configurations at TunGo's standard paths cause failure; they are never overwritten.
Success prints `PASS`; failure exits with code 1. Logs are in each workdir's `logs/`.
Deadlines: 8 minutes for the scenario, 5 for the client part.
