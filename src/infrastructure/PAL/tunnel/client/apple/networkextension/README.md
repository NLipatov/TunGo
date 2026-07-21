# TunGo Apple NetworkExtension prototype

This directory contains the native NetworkExtension side of the Apple client
adapter and the shared bridge intended for a later iOS application. It does not
provide a host application or user interface. The corresponding Go adapter is
in the parent directory, and its C archive entry point is in `../carchive`.

## Structure

- `Sources/TunGoApple` — Packet Tunnel provider, NetworkExtension settings, and
  lifecycle adapter.
- `Sources/CTunGo` — UTUN descriptor discovery used by the fd-based backend.
- `build-go-core.sh` — builds the Go backend as static Apple archives or an
  XCFramework.
- `project.yml` — XcodeGen specification for the Packet Tunnel extension.

## Local setup

1. Install full Xcode and select it with `xcode-select`.
2. Install XcodeGen.
3. Run `./build-go-core.sh macos`.
4. Run `xcodegen generate` from this directory.
5. Select a development team and enable the Network Extension capability for
   the extension's App ID.

To install and run the extension, embed `TunGoPacketTunnel.appex` into the
existing macOS host application. VPN profile management is deliberately outside
this backend-only change. The provider starts the normal TunGo client runtime,
which reads the selected configuration through the existing platform path
(`/etc/tungo/client_configuration.json` on macOS).

The fd discovery mirrors the architecture used by WireGuard's Apple client: the
provider process locates its UTUN control socket and passes the borrowed fd to
Go. The fd is registered at the `PAL/tunnel/client` boundary before the normal
client runtime is created. `NewPlatformTunManager` then uses a duplicate of that
fd instead of creating and configuring another UTUN. The runtime never closes
the descriptor owned by NetworkExtension.
