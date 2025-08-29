![Build](https://github.com/NLipatov/TunGo/actions/workflows/main.yml/badge.svg)
[![codecov](https://codecov.io/gh/NLipatov/TunGo/branch/main/graph/badge.svg)](https://codecov.io/gh/NLipatov/TunGo)
![License](https://img.shields.io/badge/license-AGPL_v3-blue.svg?style=plastic)
![Stars](https://img.shields.io/github/stars/NLipatov/TunGo.svg)
![Forks](https://img.shields.io/github/forks/NLipatov/TunGo.svg)
![Issues](https://img.shields.io/github/issues/NLipatov/TunGo.svg)

# TunGo: What's It All About?

<p align="center">
  <img alt="Two gophers dancing tango" src="https://i.ibb.co/K7yzDf6/DALL-E-2024-10-04-20-18-51-A-minimalist-logo-featuring-two-Go-language-mascots-dancing-tango-togethe.webp" width="40%"/>
</p>

**TunGo** is a lightweight VPN designed for modern needs: **fast**, **secure**, and **easy to understand**.

### Key features:
- 🚀 **High Performance** — negligible CPU usage and no allocations under load.
- 📦 **Tiny Memory Footprint** — ~8 MB idle, ~14 MB under traffic.
- 🔒 **End-to-End Encryption** — X25519 (Curve25519 ECDH) for key agreement; ChaCha20-Poly1305 (AEAD) for traffic encryption; Ed25519 for authentication.
- ⚡ **Built from Scratch** — no legacy, no bloat. Clean, readable Go code.
- 🌐 **IoT and Embedded Ready** — optimized for small devices and constrained environments.
- 🛡️ **Open Source** — AGPLv3 licensed, free for community use, commercial licenses available.

---

TunGo is engineered for developers who value **transparency**, **efficiency**, and **freedom**.  
Simple to deploy, easy to audit, ready to adapt.

---


## 📑 Table of Contents

* [License](#-license)
* [QuickStart](#-quickstart)

    * [Server Setup](#%EF%B8%8F-server-setup-linux-only)
    * [Client Setup](#%EF%B8%8F-client-setup)
* [Advanced Use Cases](#-advanced-use-cases)

---

## 📜 License

**Free for non-commercial use**

Use for personal privacy, censorship bypassing, and educational purposes.

**Commercial licensing**

This project is licensed under the [GNU AGPLv3](./LICENSE).
For commercial use, contact Nikita Lipatov at [6stringsohei@gmail.com](mailto:6stringsohei@gmail.com).

---

## 🚀 QuickStart

Get TunGo server and client up and running in minutes!

### 🖥️ Server Setup (Linux only)

1. **Install TunGo**

   ```bash
   wget https://github.com/NLipatov/TunGo/releases/latest/download/tungo-linux-amd64 -O tungo  
   chmod +x tungo  
   sudo mv tungo /usr/local/bin/
   ```

2. **Generate Client Config**

   ```bash
   sudo tungo s gen
   ```

   > Copy the displayed configuration for your client.

3. **Start Server**

   ```bash
   sudo tungo
   ```

   > In the TUI menu, select **server → start server**.

---

### 🖥️ Client Setup

#### Linux (x64)

```bash
wget https://github.com/NLipatov/TunGo/releases/latest/download/tungo-linux-amd64 -O tungo && \
chmod +x tungo && \
sudo mv tungo /usr/local/bin/
```

#### macOS (Apple Silicon)

```bash
wget https://github.com/NLipatov/TunGo/releases/latest/download/tungo-darwin-arm64 -O tungo && \
chmod +x tungo && \
sudo mv tungo /usr/local/bin/
```

#### macOS (x64)

```bash
wget https://github.com/NLipatov/TunGo/releases/latest/download/tungo-darwin-amd64 -O tungo
chmod +x tungo
sudo mv tungo /usr/local/bin/
```

#### Windows (x64)

Download the installer from the [release page](https://github.com/NLipatov/TunGo/releases).

---

**Launch Tunnel:**

* **Linux/macOS:** `sudo tungo`
* **Windows:** Run as Administrator.

🎉 You're all set!

---

## 🔧 Advanced Use Cases

See https://tungo.ethacore.com for more use cases, like:
1) [How to run server in docker container](https://tungo.ethacore.com/docs/Advanced/Containerization/Docker/Server)
2) [How to setup server systemd daemon (for automated start on system startup)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20server%20systemd%20unit)
3) [How to setup client systemd daemon (for automated start on system startup)](https://tungo.ethacore.com/docs/Advanced/Linux/Setup%20client%20systemd%20unit)
