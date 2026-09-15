#!/usr/bin/env bash
set -euo pipefail

main() {
    if [[ ${GITHUB_ACTIONS:-} != true ]]; then
        echo 'Use a disposable GitHub Actions runner' >&2
        exit 1
    fi
    if [[ $# != 3 ]]; then
        echo 'usage: run.sh <protocol> <amd64|arm64> <workdir>' >&2
        exit 1
    fi
    local protocol=$1 arch=$2
    case "$arch" in amd64|arm64) ;; *) exit 1 ;; esac
    scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
    workdir=$(cd "$3" && pwd)
    host=$(uname -s)
    case "$host" in
        MINGW*|MSYS*|CYGWIN*) workdir=$(cygpath -m "$workdir") ;;
    esac
    logs="$workdir/logs"
    mkdir -p "$logs"
    trap finish EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    prepare_ssh
    start_client_ssh
    start_server "$arch"
    wait_for_server
    local harness="$workdir/tungo-e2e"
    case "$host" in MINGW*|MSYS*|CYGWIN*) harness+=.exe ;; esac
    "$harness" coordinator "$protocol" "$workdir"
}

prepare_ssh() {
    # This directory is owned by this invocation; never reuse existing keys.
    mkdir "$workdir/tungo-ssh"
    keydir="$workdir/tungo-ssh"
    case "$host" in
        Linux|Darwin)
            bash "$scripts/unix/setup-ssh.sh" "$workdir" "${SUDO_USER:-root}"
            sshd=/usr/sbin/sshd
            ;;
        MINGW*|MSYS*|CYGWIN*)
            pwsh -NoLogo -NoProfile -NonInteractive -File "$scripts/windows/setup-ssh.ps1" -Workdir "$workdir"
            sshd="$workdir/openssh/sshd.exe"
            ;;
    esac
    read -r _ public_key _ <"$keydir/identity.pub"
}

start_client_ssh() {
    MSYS2_ARG_CONV_EXCL='*' "$sshd" -D -e -f "$keydir/sshd-client.conf" \
        >"$logs/sshd-client.log" 2>&1 &
    sshd_pid=$!
}

start_server() {
    local arch=$1 qemu_arch=x86_64 machine=q35 cpu=max console=ttyS0
    if [[ "$arch" == arm64 ]]; then
        qemu_arch=aarch64 machine=virt cpu=cortex-a72 console=ttyAMA0
    fi
    local ssh_address=192.168.250.15:22 vpn_address=192.168.250.15 network
    case "$host" in
        Linux)
            ip tuntap add dev tungo-vm0 mode tap
            tap=tungo-vm0
            ip addr add 192.168.250.1/24 dev "$tap"
            ip link set "$tap" up
            network="tap,id=transport,ifname=$tap,script=no,downscript=no"
            ;;
        Darwin)
            network=vmnet-host,id=transport,start-address=192.168.250.1,end-address=192.168.250.254,subnet-mask=255.255.255.0
            ;;
        MINGW*|MSYS*|CYGWIN*)
            ssh_address=127.0.0.1:2222 vpn_address=127.77.0.1
            # The guest firewall isolates egress; restrict=on drops UDP replies.
            # The transport is IPv4; IPv6 router advertisements would change its routes.
            network=user,id=transport,net=192.168.250.0/24,ipv6=off
            network+=,hostfwd=tcp:127.0.0.1:2222-192.168.250.15:22
            network+=,hostfwd=tcp:127.77.0.1:8080-192.168.250.15:8080
            network+=,hostfwd=udp:127.77.0.1:9090-192.168.250.15:9090
            network+=,hostfwd=tcp:127.77.0.1:1010-192.168.250.15:1010
            ;;
        *) echo "Unsupported runner: $host" >&2; exit 1 ;;
    esac
    printf '{"SSH":"%s","VPN":"%s"}\n' "$ssh_address" "$vpn_address" >"$workdir/server.json"
    # Keep MSYS from rewriting Linux kernel arguments such as rdinit=/init.
    MSYS2_ARG_CONV_EXCL='*' "qemu-system-$qemu_arch" \
        -accel tcg -m 1024 -smp 2 -display none -monitor none -no-reboot \
        -kernel "$workdir/tungo-vm/kernel" -initrd "$workdir/tungo-vm/initramfs.gz" \
        -serial stdio -append "console=$console rdinit=/init panic=1 tungo_ssh_key=$public_key" \
        -machine "$machine" -cpu "$cpu" -netdev "$network" \
        -device virtio-net-pci,netdev=transport >"$logs/vm.log" 2>&1 &
    qemu_pid=$!
}

wait_for_server() {
    local deadline=$((SECONDS + 240))
    while (( SECONDS < deadline )); do
        if ! kill -0 "$sshd_pid" 2>/dev/null; then
            echo 'Client SSH server exited before the test' >&2
            return 1
        fi
        if ! kill -0 "$qemu_pid" 2>/dev/null; then
            echo 'QEMU exited before SSH became ready' >&2
            return 1
        fi
        # Pin the guest key through the local console, never through the network.
        if [[ -f "$logs/vm.log" ]]; then
            sed -n 's/^TUNGO_E2E_SSH_HOST_KEY //p' "$logs/vm.log" >"$keydir/server.pub"
            if [[ -s "$keydir/server.pub" ]]; then
                return
            fi
        fi
        sleep 1
    done
    echo 'Timed out waiting for the server VM' >&2
    return 1
}

finish() {
    local status=$?
    trap - EXIT INT TERM
    if (( status != 0 )); then
        local log
        for log in "$logs/vm.log" "$logs/sshd-client.log"; do
            if [[ -f "$log" ]]; then
                printf '%s:\n' "$log" >&2
                tail -c 20000 "$log" >&2
            fi
        done
    fi
    local pid deadline
    for pid in "${sshd_pid:-}" "${qemu_pid:-}"; do
        [[ -n "$pid" ]] || continue
        kill "$pid" 2>/dev/null || true
        deadline=$((SECONDS + 10))
        while kill -0 "$pid" 2>/dev/null && (( SECONDS < deadline )); do sleep 1; done
        kill -KILL "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    done
    if [[ -n ${tap:-} ]]; then
        ip link delete "$tap" || status=1
    fi
    if [[ -n ${keydir:-} ]]; then
        rm -rf -- "$keydir" || status=1
    fi
    exit "$status"
}

main "$@"
