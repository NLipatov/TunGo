# Introducing TunGo

<p align="center">
  <img 
alt="2 gophers dancing a tango"
src="https://i.ibb.co/K7yzDf6/DALL-E-2024-10-04-20-18-51-A-minimalist-logo-featuring-two-Go-language-mascots-dancing-tango-togethe.webp" width="40%"/>
</p>

TunGo is a tiny, secure VPN implemented from scratch in Go.  
It uses Ed25519 for key exchange and ChaCha20 for traffic encryption between the server and client.

# Quick Start

### Run the Server

Start the server in a Docker container using the following command:

### Important: **Replace 'ServerIP' with an IP of your server**
Args:
- EnableUDP - tells server-app if UDP should be enabled (PORT 9090);
- EnableTCP - tells server-app if TCP should be enabled (PORT 8080);
- ServerIP - generated client configs will contain this IP as a server IP. 

```bash
sudo docker run --restart always -it --network host --device=/dev/net/tun --cap-add=NET_ADMIN \
    -p 8080:8080 -p 9090:9090 \
    -e EnableUDP=true  \
    -e EnableTCP=false \
    -e ServerIP="192.168.122.194" \
    -v tungo_src:/src \
    nlipatov/tungo:tungo
```
NOTE: This container has no ED25519 keys in its server conf.json, so new pair will be generated.

### Connect as a Client

First, you need to generate a client's conf via `gen` command - see `Interactive Commands` section below.
Save the generated client configuration into `/src/settings/client/conf.json` before running the client.

From `/src` run the client from the command line:
```bash
sudo go run client.go
```

# Interactive Commands

TunGo supports a few interactive commands that simplify the management of your VPN setup.

### Command: generate client configuration

While the server is running, type the gen command to generate the client configuration.
This will print out the necessary connection details.
```bash
gen
```

Example:
```
2024/10/04 20:12:13 server configured
2024/10/04 20:12:13 server listening on port :8080
gen
{
  "TCPSettings": {
    "InterfaceName": "tcptun0",
    "InterfaceIPCIDR": "10.0.0.0/24",
    "InterfaceAddress": "10.0.0.2",
    "ConnectionIP": "46.226.163.79",
    "ConnectionPort": ":8080",
    "Protocol": "tcp"
  },
  "UDPSettings": {
    "InterfaceName": "udptun0",
    "InterfaceIPCIDR": "10.0.1.0/24",
    "InterfaceAddress": "10.0.1.2",
    "ConnectionIP": "46.226.163.79",
    "ConnectionPort": ":9090",
    "Protocol": "udp"
  },
  "Ed25519PublicKey": "X7zGjLlcULRCIa4XfNm4v/RYnmN7UDgI+r1ySKs6WX4=",
  "TCPWriteChannelBufferSize": 1000,
  "Protocol": "udp"
}
```

Save the generated client configuration into `src/settings/client/conf.json`.

# Command: shutdown Server or Client

To remove all the network configuration changes and gracefully stop the server or client, use the exit command from the interactive terminal:

```bash
exit
```

Example:
```bash
exit
2024/10/04 20:34:24 Exit command received. Shutting down...
2024/10/04 20:34:24 Client is shutting down.
```
# Build the Server Container

To build the server Docker container, run the following command from the project root:

```bash
docker buildx build -t tungo-server src
```

# Regenerate Server Ed25519 Keys

To regenerate server keys, manually delete the lines containing Ed25519 keys from `src/settings/settings/conf.json`.
On the next startup, the server will generate new keys.

After regeneration, all clients need to update their configurations with the server’s new public Ed25519 key.

# Benchmarking
## Iperf2
### TCP
Server:
```bash
iperf -s -B 10.0.0.1
```
In this example `10.0.0.1` is a server's address in vpn network. 

Client:
```bash
iperf -c 10.0.0.1
```

or with 100 parallel connections for 600 seconds:

```bash
iperf -c 10.0.0.1 -P 100 -t 600
```

### UDP
Server:
```shell
iperf -s -u
```

Client (at 1GB bandwidth):
```shell
iperf -c 10.0.1.1 -u -b 1G
```