![Build](https://github.com/NLipatov/TunGo/actions/workflows/main.yml/badge.svg)
[![codecov](https://codecov.io/gh/NLipatov/TunGo/branch/main/graph/badge.svg)](https://codecov.io/gh/NLipatov/TunGo)
![License](https://img.shields.io/badge/license-MIT-blue.svg?style=plastic)
![Stars](https://img.shields.io/github/stars/NLipatov/TunGo.svg)
![Forks](https://img.shields.io/github/forks/NLipatov/TunGo.svg)
![Issues](https://img.shields.io/github/issues/NLipatov/TunGo.svg)

# TunGo: Fast & Secure VPN in Go

<p align="center">
  <img alt="Two gophers dancing tango" src="https://i.ibb.co/K7yzDf6/DALL-E-2024-10-04-20-18-51-A-minimalist-logo-featuring-two-Go-language-mascots-dancing-tango-togethe.webp" width="40%"/>
</p>

**TunGo** is a lightweight and secure VPN built from scratch in Go, using **Ed25519** for key exchange and **ChaCha20** for encryption.

---

Just a few quick notes before you continue:

    1) Encountered an issue? Feel free to create an issue.
    2) Spot something that could be improved? We'd really love to see your pull request!
    3) If you find the project useful, consider giving it a ⭐.

---

## Support us ❤️
[anon pay donation link](https://trocador.app/anonpay?ticker_to=xmr&network_to=Mainnet&address=46hGgYaPUPcaQ4Xk3UeSAaUSrGV5yHJJmEAafg92iSS28L9FwzGmuGsKqdURsbuVECVhF7bfSbEVzWL4ubDUW6jEFCGXcXh&ref=sqKNYGZbRl&direct=True&name=TunGo+developers)

[anon pay donation onion link](https://tqzngtf2hybjbexznel6dhgsvbynjzezoybvtv6iofomx7gchqfssgqd.onion/anonpay?ticker_to=xmr&network_to=Mainnet&address=46hGgYaPUPcaQ4Xk3UeSAaUSrGV5yHJJmEAafg92iSS28L9FwzGmuGsKqdURsbuVECVhF7bfSbEVzWL4ubDUW6jEFCGXcXh&ref=sqKNYGZbRl&direct=True&name=TunGo+developers)

## Usage
Run:
```bash
main.go <mode>
```
- `<mode>`: `s` (server) or `c` (client).

---

## 🚀 Quick Start
1. Start the server.
2. Generate client configuration (`gen` command).
3. Start the client with the generated configuration.

## Detailed Setup

### Start the Server
1. Launch the server:
   ```bash
   main.go s
   ```
2. Generate client configuration:
   ```bash
      gen #`gen` command
      {
        "TCPSettings": {
          "...": "..."
        },
        "UDPSettings": {
          "...": "..."
        },
        "Ed25519PublicKey": "someGeneratedPublicKey",
        "TCPWriteChannelBufferSize": 1000,
        "Protocol": "udp"
      }
   ```

3. Save the output as client conf (`settings/client/conf.json`)

4. Start the client:
   ```bash
   main.go c
   ```

✅ VPN tunnel is now established!

To stop and clean up:
```bash
exit  # from client terminal
```
OR stop client and reconfigure network
```bash
sudo ip link delete udptun0
sudo ip link delete tcptun0
```

---

## Using Docker
Run the server in Docker:
```bash
docker run -d \
  --name tungo \
  --restart always \
  --network host \
  --device /dev/net/tun \
  --cap-add NET_ADMIN \
  -e EnableUDP=true \
  -e EnableTCP=false \
  -e UDPRingBufferSize=100_000 \
  -e ED25519_PUBLIC_KEY="base64-encoded key" \
  -e ED25519_PRIVATE_KEY="base64-encoded key" \
  -v tungo_volume:/src \
  nlipatov/tungo:tungo
```
if no `ED25519_PUBLIC_KEY` and `ED25519_PRIVATE_KEY` provided, server-app will generate it on startup.

Continue with the same steps:
1. Generate client config (`gen`).
2. Save it as `client/conf.json`.
3. Start the client.

---

## 🔑 Regenerate Server Keys
To reset the server’s Ed25519 keys:
1. Remove the Ed25519 keys from `src/settings/settings/conf.json`.
2. Restart the server.

**Note**: Clients must update their configurations with the new server public key.

---

## 📊 Performance Benchmarking

### iperf2

#### TCP
**Server**:
```bash
iperf -s -B 10.0.0.1
```

**Client**:
```bash
iperf -c 10.0.0.1
```

For parallel connections:
```bash
iperf -c 10.0.0.1 -P 100 -t 600
```

#### UDP
**Server**:
```bash
iperf -s -u
```

**Client** (1GB bandwidth):
```bash
iperf -c 10.0.1.1 -u -b 1G
```

### iperf3

#### TCP
**Server**:
```bash
iperf3 -s -B 10.0.0.1
```

**Client**:
```bash
iperf3 -c 10.0.0.1
```

For parallel connections:
```bash
iperf3 -c 10.0.0.1 -P 100 -t 600
```

#### UDP
**Server**:
```bash
iperf3 -s -u
```

**Client** (1GB bandwidth):
```bash
iperf3 -c 10.0.1.1 -u -b 1G
```

---

## Debugging with bubble tea
see: https://github.com/charmbracelet/bubbletea?tab=readme-ov-file#debugging-with-delve
```bash
sudo dlv debug --headless --listen=:2345 --api-version=2 --log --check-go-version=false
```

Start enjoying fast and secure tunneling with **TunGo**!
