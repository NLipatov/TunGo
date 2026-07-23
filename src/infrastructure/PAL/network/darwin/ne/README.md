# TunGo Apple NetworkExtension adapter

This directory contains the native NetworkExtension side of the Apple client
adapter and the shared bridge for macOS and iOS. The containing application
project lives in `presentation/ui/gui/apple`.

## Structure

- `Sources/TunGoApple` — Packet Tunnel provider, NetworkExtension settings, and
  lifecycle adapter.
- `Sources/CTunGo` — UTUN descriptor discovery used by the fd-based backend.
- `carchive` — the narrow C ABI between Swift and the Go client runtime.
- `manager` — adapts the NetworkExtension-owned UTUN descriptor to `tun.ClientManager`.
- `build-go-core.sh` — builds the Go backend as static Apple archives or an
  XCFramework.

## Local setup

1. Install full Xcode and select it with `xcode-select`.
2. Install XcodeGen.
3. Run `./build-go-core.sh macos`.
4. Run `xcodegen generate` from `presentation/ui/gui/apple`.
5. Select a development team and enable the Network Extension capability for
   the extension's App ID.

The provider starts the normal TunGo client runtime, which reads the selected
configuration through the existing platform path
(`/etc/tungo/client_configuration.json` on macOS).

The fd discovery mirrors the architecture used by WireGuard's Apple client: the
provider process locates its UTUN control socket and passes the borrowed fd to
Go. The fd is registered in `PAL/network/darwin/ne` before the normal client
runtime is created. The composition point in `PAL/tunnel/client` then selects
the NE manager and uses a duplicate of that fd instead of creating and
configuring another UTUN. The runtime never closes the descriptor owned by
NetworkExtension.
