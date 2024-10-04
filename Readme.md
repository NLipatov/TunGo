# Introducing TunGo

<p align="center">
  <img 
alt="2 gophers dancing a tango"
src="https://i.ibb.co/K7yzDf6/DALL-E-2024-10-04-20-18-51-A-minimalist-logo-featuring-two-Go-language-mascots-dancing-tango-togethe.webp" width="40%"/>
</p>

Tiny, secure VPN implemented from scratch in Go.  
TunGo uses Ed25519 for key exchange and ChaCha20 to encrypt traffic between server and client.

# Usage

### Run the server in a Docker container:
```bash
docker run -it --device=/dev/net/tun --cap-add=NET_ADMIN -p 8080:8080 nlipatov/tungo:tungo-server
```

Connect to the server as a client:

```bash
sudo go run client.go
```

# Regenerate Server Ed25519 Keys

To regenerate server Ed25519 keys, remove the key lines from src/settings/settings/conf.json.
The server will generate new keys on the next startup.

After regeneration, each client must update its configuration with the server’s new public Ed25519 key.

# Generate Client Configuration

Run the following command to generate the client configuration:

```bash
sudo go run server.go gen
```

Example output:
```
json

{
  "IfName": "ethatun0",
  "IfIP": "10.0.0.3/24",
  "ServerTCPAddress": "192.168.122.194:8080",
  "Ed25519PublicKey": "m+tjQmYAG8tYt8xSTry29Mrl9SInd9pvoIsSywzPzdU="
}
```

Save this configuration in src/settings/client/conf.json.

# Build the Server Container

```bash
docker buildx build -t tungo-server src
```
