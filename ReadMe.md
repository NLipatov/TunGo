![Build](https://github.com/NLipatov/TunGo/actions/workflows/main.yml/badge.svg)
[![codecov](https://codecov.io/gh/NLipatov/TunGo/branch/main/graph/badge.svg)](https://codecov.io/gh/NLipatov/TunGo)
![License](https://img.shields.io/badge/license-MIT-blue.svg?style=plastic)
![Stars](https://img.shields.io/github/stars/NLipatov/TunGo.svg)
![Forks](https://img.shields.io/github/forks/NLipatov/TunGo.svg)
![Issues](https://img.shields.io/github/issues/NLipatov/TunGo.svg)

# License

This project is licensed under the GNU Affero General Public License v3.0 (AGPLv3).  
See [LICENSE](./LICENSE) for details.

For commercial licensing inquiries, please contact:

Nikita Lipatov

Email: 6stringsohei@gmail.com

# Project Documentation

## Quickstart
See the Quickstart guide: https://tungo.ethacore.com/docs/quickstart

---

## Use Docker to deploy server
Refer to https://tungo.ethacore.com/docs/Advanced/Containerization/Docker/Server

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

---
## Support us ❤️
[anon pay donation link](https://trocador.app/anonpay?ticker_to=xmr&network_to=Mainnet&address=46hGgYaPUPcaQ4Xk3UeSAaUSrGV5yHJJmEAafg92iSS28L9FwzGmuGsKqdURsbuVECVhF7bfSbEVzWL4ubDUW6jEFCGXcXh&ref=sqKNYGZbRl&direct=True&name=TunGo+developers)

[anon pay donation onion link](https://tqzngtf2hybjbexznel6dhgsvbynjzezoybvtv6iofomx7gchqfssgqd.onion/anonpay?ticker_to=xmr&network_to=Mainnet&address=46hGgYaPUPcaQ4Xk3UeSAaUSrGV5yHJJmEAafg92iSS28L9FwzGmuGsKqdURsbuVECVhF7bfSbEVzWL4ubDUW6jEFCGXcXh&ref=sqKNYGZbRl&direct=True&name=TunGo+developers)