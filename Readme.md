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

```bash
docker run -it --device=/dev/net/tun --cap-add=NET_ADMIN -p 8080:8080 nlipatov/tungo:tungo-server
```
NOTE: This container has no ED25519 keys in its server conf.json, so new pair will be generated.

### Connect as a Client

Run the client from the command line:

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
  "IfName": "ethatun0",
  "IfIP": "10.0.0.4/24",
  "ServerTCPAddress": "192.168.122.194:8080",
  "Ed25519PublicKey": "PSGbN32XBr+foaD5HkZatNqigTfpqUlbdYBOCNXjtBo="
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
