![Build](https://github.com/NLipatov/TunGo/actions/workflows/main.yml/badge.svg)
[![codecov](https://codecov.io/gh/NLipatov/TunGo/branch/main/graph/badge.svg)](https://codecov.io/gh/NLipatov/TunGo)
[![License](https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg?style=plastic)](./LICENSE)
![Stars](https://img.shields.io/github/stars/NLipatov/TunGo.svg)
![Forks](https://img.shields.io/github/forks/NLipatov/TunGo.svg)
![Issues](https://img.shields.io/github/issues/NLipatov/TunGo.svg)

# TunGo: What's It All About?

<p align="center">
  <img alt="Two gophers dancing tango" src="https://i.ibb.co/GvSCJ01n/Tun-Go-logo.png" width="40%"/>
</p>

**TunGo** is a lightweight VPN designed for modern needs: **fast**, **secure**, and **open-source**.

### Key features:
- 🧩 **Easy to deploy & configure**.
- 🚀 **High Performance** — near-zero allocations on the hot path (benchmarked).
- 📦 **Tiny Memory Footprint** — ≈5–15 MB **RSS** under load, ≈5–8 MB idle.
- 🔒 **End-to-End Encryption** — Noise IK handshake for mutual authentication; X25519 (Curve25519 ECDH) for key agreement; ChaCha20-Poly1305 (AEAD) for traffic encryption.
- ⚡ **Built from Scratch** — no legacy, no bloat. Clean, readable Go code.
- 🌐 **IoT & Embedded Ready** — optimized for small devices and constrained environments.
- 🛡️ **Open Source** — AGPL-3.0-only; commercial licenses available.

---

## 🚀 QuickStart

Refer to: [QuickStart](https://tungo.ethacore.com/docs/QuickStart)

---

## 🔧 Advanced Use Cases

See more use cases at https://tungo.ethacore.com, for example:

1) [How to run the server in a Docker container](https://tungo.ethacore.com/docs/Advanced/Containerization/Docker/Server)
2) [How to set up a server systemd unit (auto-start on boot)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20server%20systemd%20unit)
3) [How to set up a client systemd unit (auto-start on boot)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20client%20systemd%20unit)

---

## 📜 License — AGPL-3.0-only

**TL;DR:** You can use TunGo privately or commercially.

If users interact with your (modified or unmodified) TunGo over a network (SaaS/hosted), you must provide them the complete corresponding source code of TunGo, including your changes and the scripts used to control compilation and installation.  
Independent services that only communicate with TunGo over standard protocols (HTTP/gRPC, queues, etc.) do not have to be open-sourced (unless they are part of a derivative work).

### 🔒 Need a closed integration?
If you need to **embed/link** TunGo into a proprietary product without sharing source, contact <mailto:6stringsohei@gmail.com> for a **commercial license**.

### ✅ You can
- Run TunGo for personal or commercial use.
- Modify it and keep changes private **as long as no users access it over a network**.
- Host it as a paid service (SaaS).
- Combine it with separate services via clean network boundaries.

### 🧾 You must (when applicable)
- For hosted/SaaS use: offer users a link to download the **source of the TunGo version you run**, incl. your patches and build/install scripts.
- For binary distribution: ship or offer the source for the distributed TunGo parts.
- Keep copyright notices and the AGPL-3.0 license text.

### ❗ You don’t have to
- Open-source unrelated services, databases, infra, or monitoring—unless they become a **derivative work** of TunGo.
