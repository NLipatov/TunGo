![Build](https://github.com/NLipatov/TunGo/actions/workflows/main.yml/badge.svg)
[![codecov](https://codecov.io/gh/NLipatov/TunGo/branch/main/graph/badge.svg)](https://codecov.io/gh/NLipatov/TunGo)
[![License](https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg?style=plastic)](./LICENSE)
![Stars](https://img.shields.io/github/stars/nlipatov/tungo.svg)
![Forks](https://img.shields.io/github/forks/NLipatov/TunGo.svg)
![Issues](https://img.shields.io/github/issues/NLipatov/TunGo.svg)

# TunGo: What's It All About?

<p align="center">
  <img alt="Two gophers dancing tango" src="https://i.ibb.co/GvSCJ01n/Tun-Go-logo.png" width="40%"/>
</p>

**TunGo** is a lightweight VPN designed for modern needs: **fast**, **secure**, and **open-source**.

## Features

- UDP, TCP and WebSocket transports
- **0 allocs/packet** on the dataplane hot path
- Interactive TUI for Linux, macOS and Windows

---

## 📈 Performance

TunGo includes in-memory full-cycle dataplane benchmarks for both UDP and TCP. These benchmarks measure userspace packet-processing throughput only: encryption, routing/lookup, validation, decryption, and handoff to an in-memory sink.

Example single-run results for **1400-byte packets** on **Apple M4 Pro**:

| Path | ns/op | Throughput | Allocs/op |
|---|---:|---:|---:|
| UDP client -> server | ~2.7 us | ~4.3 Gbit/s | 0 |
| UDP server -> client | ~2.6 us | ~4.3 Gbit/s | 0 |
| TCP client -> server | ~2.6 us | ~4.3 Gbit/s | 0 |
| TCP server -> client | ~2.6 us | ~4.3 Gbit/s | 0 |

These numbers do **not** include TUN device, socket, kernel, firewall/NAT, or real network overhead. Treat them as dataplane-core benchmarks, not end-to-end VPN throughput claims.

To reproduce:

```bash
cd src
go test ./infrastructure/tunnel/dataplane/server/udp_chacha20 ./infrastructure/tunnel/dataplane/client/udp_chacha20 ./infrastructure/tunnel/dataplane/server/tcp_chacha20 ./infrastructure/tunnel/dataplane/client/tcp_chacha20 -run ^$ -bench FullCycle -benchmem
```

---

## 🚀 QuickStart

Refer to: [QuickStart](https://tungo.ethacore.com/docs/QuickStart)

---

## 🔧 Advanced Use Cases

See more use cases at https://tungo.ethacore.com, for example:

1) [How to run the server in a Docker container](https://tungo.ethacore.com/docs/Advanced/Containerization/Docker/Server)
2) [How to set up a server systemd unit (auto-start on boot)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20server%20systemd%20unit)
3) [How to set up a client systemd unit (auto-start on boot)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20client%20systemd%20unit)
