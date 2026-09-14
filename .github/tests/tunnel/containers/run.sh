#!/usr/bin/env bash
set -Eeuo pipefail

protocol=${1:-UDP}
case "$protocol" in
  UDP)
    enable_udp=true
    enable_tcp=false
    enable_ws=false
    ;;
  TCP)
    enable_udp=false
    enable_tcp=true
    enable_ws=false
    ;;
  WS)
    enable_udp=false
    enable_tcp=false
    enable_ws=true
    ;;
  *)
    echo "Usage: bash $0 {UDP|TCP|WS}" >&2
    exit 2
    ;;
esac

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)
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
tunnel_ip=([4]=198.19.0.1 [6]=fd73:7467:6f:1::1)
declare -a target_ip server_backend_ip expected_hash

assert_no_bypass() {
  # Also prove the HTTP fixture is alive, so its failure cannot make this pass.
  local family
  for family in 4 6; do
    http_get "$server" "$family" sha256 >/dev/null
    if http_get "$client" "$family" sha256 >/dev/null 2>&1; then
      fail "IPv$family client reached the target without a tunnel"
    fi
    if docker exec "$client" ip "-$family" route get "${target_ip[family]}" >/dev/null 2>&1; then
      fail "IPv$family client has an unexpected route to the isolated target"
    fi
  done
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

assert_traffic() {
  local family=$1
  local route prefix actual_hash
  local prefixes=(0.0.0.0/1 128.0.0.0/1)
  if [[ $family == 6 ]]; then prefixes=(::/1 8000::/1); fi
  echo "Checking IPv$family traffic"
  wait_until "IPv$family traffic through $protocol" http_get "$client" "$family" sha256
  route=$(docker exec "$client" ip "-$family" route get "${target_ip[family]}")
  [[ " $route " == *" dev $client_tun "* ]] || fail "IPv$family target route bypasses TunGo: $route"
  for prefix in "${prefixes[@]}"; do
    route=$(docker exec "$client" ip "-$family" route show exact "$prefix")
    [[ " $route " == *" dev $client_tun "* ]] || fail "Missing tunnel route $prefix on $client_tun"
  done
  docker exec "$client" ping "-$family" -n -c 3 -W 2 "${tunnel_ip[family]}"
  http_get "$client" "$family" payload --output /tmp/tungo-payload
  actual_hash=$(docker exec "$client" sha256sum /tmp/tungo-payload)
  [[ ${actual_hash%% *} == "${expected_hash[family]}" ]] || fail "IPv$family payload checksum mismatch"
  [[ $(http_get "$client" "$family" peer) == "${server_backend_ip[family]}" ]] || fail "IPv$family traffic did not use server MASQUERADE"
}

stop_tungo() {
  local container=$1
  docker exec "$container" sh -c 'kill -TERM "$(cat /tmp/tungo.pid)"'
  wait_until "$container graceful shutdown" docker exec "$container" test -s /tmp/tungo.status
  [[ $(docker exec "$container" cat /tmp/tungo.status) == 0 ]] || fail "$container did not exit cleanly"
}

assert_restored() {
  local container=$1
  local name=$2
  local phase=$3
  local tun=$4
  local family
  if docker exec "$container" ip link show dev "$tun" >/dev/null 2>&1; then
    fail "$container left $tun behind"
  fi
  snapshot "$container" "$name.$phase"
  for family in 4 6; do
    diff -u "$artifact_dir/$name.before.routes$family" "$artifact_dir/$name.$phase.routes$family"
    diff -u "$artifact_dir/$name.before.firewall$family" "$artifact_dir/$name.$phase.firewall$family"
  done
}

finish() {
  local status=$?
  trap - EXIT
  set +e
  for container in "$client" "$server" "$target"; do
    docker logs "$container" >"$artifact_dir/$container.docker.log" 2>&1
    docker exec "$container" cat /tmp/tungo.log >"$artifact_dir/$container.tungo.log" 2>&1
    docker exec "$container" sh -c '
      ip -details link show
      ip -4 route show table all
      ip -6 route show table all
      iptables-save
      ip6tables-save
      cat /tmp/tungo.status 2>/dev/null
    ' >"$artifact_dir/$container.network.txt" 2>&1
  done
  docker rm -f "$client" "$server" "$target" >/dev/null 2>&1
  docker network rm "$transport" "$backend" >/dev/null 2>&1
  docker image rm "$test_image" >/dev/null 2>&1
  echo "Diagnostics: $artifact_dir"
  exit "$status"
}

container_ip() {
  local field=IPAddress
  if [[ $3 == 6 ]]; then field=GlobalIPv6Address; fi
  docker inspect --format "{{(index .NetworkSettings.Networks \"$2\").$field}}" "$1"
}

snapshot() {
  local container=$1
  local name=$2
  local family firewall
  for family in 4 6; do
    docker exec "$container" ip "-$family" route show table main | LC_ALL=C sort >"$artifact_dir/$name.routes$family"
    firewall=iptables
    if [[ $family == 6 ]]; then firewall=ip6tables; fi
    docker exec "$container" sh -ec '
      for table in filter nat mangle; do "$1" -t "$table" -S; done
    ' sh "$firewall" >"$artifact_dir/$name.firewall$family"
  done
}

wait_until() {
  local description=$1
  local deadline=$((SECONDS + 30))
  shift
  until "$@" >/dev/null 2>&1; do
    ((SECONDS < deadline)) || fail "Timed out waiting for $description"
    sleep 0.25
  done
}

http_get() {
  local container=$1
  local family=$2
  local path=$3
  local address=${target_ip[family]}
  if [[ $family == 6 ]]; then address="[$address]"; fi
  shift 3
  docker exec "$container" curl --noproxy '*' --fail --silent --show-error \
    --connect-timeout 2 --max-time 10 "$@" "http://$address:8080/$path"
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker info >/dev/null
case $(docker info --format '{{.Architecture}}') in
  x86_64 | amd64) native_arch=amd64 ;;
  aarch64 | arm64) native_arch=arm64 ;;
  *) fail "This test supports native Linux amd64 and arm64 Docker hosts" ;;
esac
[[ $native_arch == "${EXPECTED_ARCH:-$native_arch}" ]] || fail "Docker host architecture does not match EXPECTED_ARCH"
go_version=$(awk '$1 == "go" { print $2; exit }' "$repo_root/src/go.mod")
[[ -n $go_version ]] || fail "Missing Go version in src/go.mod"
echo "Building native $native_arch TunGo and test tools for $protocol"
docker build --build-arg "GO_VERSION=$go_version" --tag "$test_image" \
  --file "$repo_root/.github/tests/tunnel/containers/Dockerfile" "$repo_root" 2>&1 | tee "$artifact_dir/build.log"
[[ $(docker image inspect --format '{{.Architecture}}' "$test_image") == "$native_arch" ]] || fail "Test image architecture does not match Docker host"

# Internal dual-stack bridges have no default route. Only the server joins both networks.
docker network create --internal --ipv6 "$transport" >/dev/null
docker network create --internal --ipv6 "$backend" >/dev/null
# Container sysctls are read-only after creation; VM tests cover TunGo enabling forwarding.
docker run -d --name "$server" --network "$transport" \
  --cap-add NET_ADMIN --device /dev/net/tun --sysctl net.ipv4.ip_forward=1 \
  --sysctl net.ipv6.conf.all.forwarding=1 --sysctl net.ipv6.conf.default.forwarding=1 \
  -e "EnableUDP=$enable_udp" -e "EnableTCP=$enable_tcp" -e "EnableWS=$enable_ws" \
  "$test_image" >/dev/null
docker network connect "$backend" "$server"
docker run -d --name "$client" --network "$transport" \
  --cap-add NET_ADMIN --device /dev/net/tun "$test_image" >/dev/null
docker run -d --name "$target" --network "$backend" \
  "$test_image" sh -c 'tungo-e2e target & exec tungo-e2e target --bind ::' >/dev/null

server_ip=$(container_ip "$server" "$transport" 4)
for family in 4 6; do
  server_backend_ip[family]=$(container_ip "$server" "$backend" "$family")
  target_ip[family]=$(container_ip "$target" "$backend" "$family")
  [[ -n ${server_backend_ip[family]} && -n ${target_ip[family]} ]] || fail "Missing IPv$family backend addresses"
done

# TunGo selects its NAT interface from the default route. Point it at the target.
docker exec "$server" ip -4 route replace default via "${target_ip[4]}"
docker exec "$server" iptables -P FORWARD DROP
docker exec "$server" ip6tables -P FORWARD DROP
# Enforce the fixture's no-bypass invariant even if Docker defaults change.
for family in 4 6; do
  docker exec "$client" ip "-$family" route flush default
  wait_until "IPv$family HTTP fixture" http_get "$server" "$family" sha256
  expected_hash[family]=$(http_get "$server" "$family" sha256)
done
snapshot "$server" server.before
snapshot "$client" client.before
assert_no_bypass

# Generate fresh keys and enroll the client through the public CLI, before startup.
docker exec -i "$server" sh -c 'mkdir -p /etc/tungo; umask 077; cat > /etc/tungo/server_configuration.json' <<EOF
{"${protocol}Settings":{"IPv4Subnet":"198.19.0.0/24","IPv6Subnet":"fd73:7467:6f:1::/64"}}
EOF
docker exec -e "Host=$server_ip" "$server" tungo s gen >/dev/null
docker exec "$client" mkdir -p /etc/tungo
docker exec "$server" cat /etc/tungo/client_configuration.json.1 \
  | docker exec -i "$client" sh -c 'umask 077; cat > /etc/tungo/client_configuration.json'
start_tungo "$server" s
wait_until "server listener" docker exec "$server" grep -q 'server listening' /tmp/tungo.log

# A second cycle tests a fresh connection after graceful teardown in the same netns.
for cycle in 1 2; do
  echo "Testing $protocol tunnel, connection $cycle"
  start_tungo "$client" c
  for family in 4 6; do assert_traffic "$family"; done
  snapshot "$client" "client.connected-$cycle"
  snapshot "$server" "server.connected-$cycle"
  stop_tungo "$client"
  assert_restored "$client" client "after-$cycle" "$client_tun"
  assert_no_bypass
done

stop_tungo "$server"
assert_restored "$server" server after "$server_tun"
assert_no_bypass
echo "PASS: $protocol IPv4/IPv6 real TUN traffic, checksum, NAT, reconnect and graceful cleanup"
