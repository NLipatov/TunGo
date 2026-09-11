#!/usr/bin/env bash
set -Eeuo pipefail

protocol=${1:-UDP}
case "$protocol" in
  UDP) enable_udp=true; enable_tcp=false; enable_ws=false; tunnel_ip=10.0.1.1 ;;
  TCP) enable_udp=false; enable_tcp=true; enable_ws=false; tunnel_ip=10.0.0.1 ;;
  WS) enable_udp=false; enable_tcp=false; enable_ws=true; tunnel_ip=10.0.2.1 ;;
  *) echo "Usage: bash $0 {UDP|TCP|WS}" >&2; exit 2 ;;
esac

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
artifact_dir=${ARTIFACT_DIR:-$(mktemp -d -t tungo-e2e-artifacts.XXXXXX)}
mkdir -p "$artifact_dir"
artifact_dir=$(cd "$artifact_dir" && pwd)
prefix="tungo-e2e-${protocol,,}-$$-$RANDOM"
transport="$prefix-transport"
backend="$prefix-backend"
server="$prefix-server"
client="$prefix-client"
target="$prefix-target"
test_image="$prefix:test"
client_tun="c_${protocol,,}tun0"
server_tun="s_${protocol,,}tun0"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

finish() {
  local status=$?
  trap - EXIT
  set +e
  for container in "$client" "$server" "$target"; do
    docker logs "$container" > "$artifact_dir/$container.docker.log" 2>&1
    docker exec "$container" cat /tmp/tungo.log > "$artifact_dir/$container.tungo.log" 2>&1
    docker exec "$container" sh -c '
      ip -details link show
      ip -4 route show table all
      iptables-save
      cat /tmp/tungo.status 2>/dev/null
    ' > "$artifact_dir/$container.network.txt" 2>&1
  done
  docker rm -f "$client" "$server" "$target" >/dev/null 2>&1
  docker network rm "$transport" "$backend" >/dev/null 2>&1
  docker image rm "$test_image" >/dev/null 2>&1
  echo "Diagnostics: $artifact_dir"
  exit "$status"
}
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_until() {
  local description=$1
  local deadline=$((SECONDS + 30))
  shift
  until "$@" >/dev/null 2>&1; do
    (( SECONDS < deadline )) || fail "Timed out waiting for $description"
    sleep 0.25
  done
}

container_ip() {
  docker inspect --format "{{(index .NetworkSettings.Networks \"$2\").IPAddress}}" "$1"
}

http_get() {
  local container=$1
  local path=$2
  shift 2
  docker exec "$container" curl --noproxy '*' --fail --silent --show-error \
    --connect-timeout 2 --max-time 10 "$@" "http://$target_ip:8080/$path"
}

assert_no_bypass() {
  # Also prove the HTTP fixture is alive, so its failure cannot make this pass.
  http_get "$server" sha256 >/dev/null
  if http_get "$client" sha256 >/dev/null 2>&1; then
    fail "Client reached the target without a tunnel"
  fi
  if docker exec "$client" ip -4 route get "$target_ip" >/dev/null 2>&1; then
    fail "Client has an unexpected route to the isolated target"
  fi
}

snapshot() {
  local container=$1
  local name=$2
  docker exec "$container" ip -4 route show table main | LC_ALL=C sort > "$artifact_dir/$name.routes"
  docker exec "$container" sh -c '
    iptables -S
    iptables -t nat -S
    iptables -t mangle -S
  ' > "$artifact_dir/$name.firewall"
}

assert_restored() {
  local container=$1
  local name=$2
  local phase=$3
  local tun=$4
  if docker exec "$container" ip link show dev "$tun" >/dev/null 2>&1; then
    fail "$container left $tun behind"
  fi
  snapshot "$container" "$name.$phase"
  diff -u "$artifact_dir/$name.before.routes" "$artifact_dir/$name.$phase.routes"
  diff -u "$artifact_dir/$name.before.firewall" "$artifact_dir/$name.$phase.firewall"
}

start_tungo() {
  local container=$1
  local mode=$2
  docker exec "$container" rm -f /tmp/tungo.pid /tmp/tungo.status
  # Keep a wrapper to record the real exit code; the container itself stays alive.
  docker exec -d "$container" sh -c '
    tungo "$1" >> /tmp/tungo.log 2>&1 &
    pid=$!
    echo "$pid" > /tmp/tungo.pid
    wait "$pid"
    echo "$?" > /tmp/tungo.status
  ' sh "$mode"
  wait_until "$container process startup" docker exec "$container" test -s /tmp/tungo.pid
}

stop_tungo() {
  local container=$1
  docker exec "$container" sh -c 'kill -TERM "$(cat /tmp/tungo.pid)"'
  wait_until "$container graceful shutdown" docker exec "$container" test -s /tmp/tungo.status
  [[ $(docker exec "$container" cat /tmp/tungo.status) == 0 ]] || fail "$container did not exit cleanly"
}

docker info >/dev/null
case $(docker info --format '{{.Architecture}}') in
  x86_64|amd64) native_arch=amd64 ;;
  aarch64|arm64) native_arch=arm64 ;;
  *) fail "This test supports native Linux amd64 and arm64 Docker hosts" ;;
esac
[[ $native_arch == "${EXPECTED_ARCH:-$native_arch}" ]] || fail "Docker host architecture does not match EXPECTED_ARCH"
go_version=$(awk '$1 == "go" { print $2; exit }' "$repo_root/src/go.mod")
[[ -n $go_version ]] || fail "Missing Go version in src/go.mod"
echo "Building native $native_arch TunGo and test tools for $protocol"
docker build --build-arg "GO_VERSION=$go_version" --tag "$test_image" \
  --file "$repo_root/.github/tests/tunnel/Dockerfile" "$repo_root" 2>&1 | tee "$artifact_dir/build.log"
[[ $(docker image inspect --format '{{.Architecture}}' "$test_image") == "$native_arch" ]] || fail "Test image architecture does not match Docker host"

# Internal bridges have no default route. Only the server joins both networks.
docker network create --internal "$transport" >/dev/null
docker network create --internal "$backend" >/dev/null
docker run -d --name "$server" --network "$transport" \
  --cap-add NET_ADMIN --device /dev/net/tun --sysctl net.ipv4.ip_forward=1 \
  -e "EnableUDP=$enable_udp" -e "EnableTCP=$enable_tcp" -e "EnableWS=$enable_ws" \
  "$test_image" >/dev/null
docker network connect "$backend" "$server"
docker run -d --name "$client" --network "$transport" \
  --cap-add NET_ADMIN --device /dev/net/tun "$test_image" >/dev/null
docker run -d --name "$target" --network "$backend" \
  "$test_image" python3 /opt/tungo-e2e/http_server.py >/dev/null

server_ip=$(container_ip "$server" "$transport")
server_backend_ip=$(container_ip "$server" "$backend")
target_ip=$(container_ip "$target" "$backend")
backend_gateway=$(docker network inspect --format '{{(index .IPAM.Config 0).Gateway}}' "$backend")

# TunGo selects its NAT interface from the default route. Point it at the target.
docker exec "$server" ip -4 route replace default via "$backend_gateway"
docker exec "$server" iptables -P FORWARD DROP
# Enforce the fixture's no-bypass invariant even if Docker defaults change.
docker exec "$client" ip -4 route flush default
wait_until "HTTP fixture" http_get "$server" sha256
expected_hash=$(http_get "$server" sha256)
snapshot "$server" server.before
snapshot "$client" client.before
assert_no_bypass

# Generate fresh keys and enroll the client through the public CLI, before startup.
docker exec -e "Host=$server_ip" "$server" tungo s gen >/dev/null
docker exec "$client" mkdir -p /etc/tungo
docker exec "$server" cat /etc/tungo/client_configuration.json.1 | \
  docker exec -i "$client" sh -c 'umask 077; cat > /etc/tungo/client_configuration.json'
start_tungo "$server" s
wait_until "server listener" docker exec "$server" grep -q 'server listening' /tmp/tungo.log

# A second cycle tests a fresh connection after graceful teardown in the same netns.
for cycle in 1 2; do
  echo "Testing $protocol tunnel, connection $cycle"
  start_tungo "$client" c
  wait_until "traffic through $protocol" http_get "$client" sha256
  docker exec "$client" ping -n -c 3 -W 2 "$tunnel_ip"
  http_get "$client" payload --output /tmp/tungo-payload
  actual_hash=$(docker exec "$client" sha256sum /tmp/tungo-payload)
  [[ ${actual_hash%% *} == "$expected_hash" ]] || fail "Payload checksum mismatch"
  [[ $(http_get "$client" peer) == "$server_backend_ip" ]] || fail "Traffic did not use server MASQUERADE"
  snapshot "$client" "client.connected-$cycle"
  snapshot "$server" "server.connected-$cycle"
  stop_tungo "$client"
  assert_restored "$client" client "after-$cycle" "$client_tun"
  assert_no_bypass
done

stop_tungo "$server"
assert_restored "$server" server after "$server_tun"
assert_no_bypass
echo "PASS: $protocol real TUN traffic, checksum, NAT, reconnect and graceful cleanup"
