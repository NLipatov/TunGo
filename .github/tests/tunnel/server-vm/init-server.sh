#!/bin/sh
set -eu
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mount -t tmpfs tmpfs /run
mkdir -p /dev/net /run/netns /run/sshd /run/tungo-e2e
for module in virtio_pci virtio_net tun veth ipv6 ip6_tables ip_tables iptable_filter iptable_nat xt_MASQUERADE xt_state; do
    modprobe "$module" || true
done
ip link set lo up
ip link set eth0 up
ip addr add 192.168.250.15/24 dev eth0

# The target has no default route: only server-side NAT can make it reply.
ip netns add target
ip link add backend0 type veth peer name target0
ip link set target0 netns target
ip addr add 198.18.0.1/24 dev backend0
ip -6 addr add fd73:7467:6f::1/64 dev backend0 nodad
ip link set backend0 up
ip netns exec target ip link set lo up
ip netns exec target ip addr add 198.18.0.2/24 dev target0
ip netns exec target ip -6 addr add fd73:7467:6f::2/64 dev target0 nodad
ip netns exec target ip link set target0 up
ip route add default via 198.18.0.2 dev backend0
# TunGo must enable kernel forwarding before either tunnel family can pass traffic.
sysctl -w net.ipv4.ip_forward=0
sysctl -w net.ipv6.conf.all.forwarding=0
iptables -P FORWARD DROP
ip6tables -P FORWARD DROP
# A host route via the guest must never bypass TunGo's TUN interface.
iptables -A FORWARD -i eth0 -o backend0 -j DROP
ip6tables -A FORWARD -i eth0 -o backend0 -j DROP
# Restrict new guest egress without libslirp restrict=on, which drops UDP replies.
# Keep IPv6 neighbor discovery working on the transport interface.
ip6tables -A OUTPUT -o eth0 -p ipv6-icmp --icmpv6-type 135 -j ACCEPT
ip6tables -A OUTPUT -o eth0 -p ipv6-icmp --icmpv6-type 136 -j ACCEPT
for firewall in iptables ip6tables; do
    "$firewall" -A OUTPUT -o eth0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    "$firewall" -A OUTPUT -o eth0 -j DROP
done
ip netns exec target nginx -g 'daemon off;' >/tmp/target.log 2>&1 &
# The runner passes only its ephemeral public key through the kernel command line.
umask 077
# Kernel arguments are words on a single line.
# shellcheck disable=SC2013
for arg in $(cat /proc/cmdline); do
    case "$arg" in
    tungo_ssh_key=*) printf 'ssh-ed25519 %s\n' "${arg#tungo_ssh_key=}" >/run/tungo-e2e/authorized_keys ;;
    esac
done
test -s /run/tungo-e2e/authorized_keys
ssh-keygen -q -t ed25519 -N '' -f /run/tungo-e2e/ssh_host_ed25519_key
# Unlock root for public-key login; password authentication stays disabled.
passwd -d root >/dev/null
cat >/run/tungo-e2e/sshd_config <<'EOF'
ListenAddress 192.168.250.15
HostKey /run/tungo-e2e/ssh_host_ed25519_key
AuthorizedKeysFile /run/tungo-e2e/authorized_keys
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
DisableForwarding yes
PermitTTY no
PrintMotd no
EOF
/usr/sbin/sshd -f /run/tungo-e2e/sshd_config -E /tmp/sshd.log
# Pin the SSH host key through QEMU's local serial output, before connecting.
printf 'TUNGO_E2E_SSH_HOST_KEY '
cat /run/tungo-e2e/ssh_host_ed25519_key.pub
echo 'TUNGO_E2E_VM_BOOTED'
# Keep SSH failures visible even when the harness cannot connect to the guest.
tail -n +1 -F /tmp/sshd.log &
while true; do sleep 3600; done
