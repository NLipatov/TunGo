#!/bin/sh
set -eu
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mkdir -p /dev/net /run/netns
mount -t tmpfs tmpfs /run
for module in virtio_pci virtio_net tun veth ip_tables iptable_filter iptable_nat xt_MASQUERADE xt_state; do
    modprobe "$module" || true
done
ip link set lo up
ip link set eth0 up
ip addr add 192.0.2.15/24 dev eth0

# The target has no default route: only server-side NAT can make it reply.
ip netns add target
ip link add backend0 type veth peer name target0
ip link set target0 netns target
ip addr add 198.18.0.1/24 dev backend0
ip link set backend0 up
ip netns exec target ip link set lo up
ip netns exec target ip addr add 198.18.0.2/24 dev target0
ip netns exec target ip link set target0 up
ip route add default via 198.18.0.2 dev backend0
sysctl -w net.ipv4.ip_forward=1
iptables -P FORWARD DROP
# A host route via the guest must never bypass TunGo's TUN interface.
iptables -A FORWARD -i eth0 -o backend0 -j DROP
ip netns exec target python3 /opt/tungo-e2e/http_server.py >/tmp/target.log 2>&1 &
python3 /opt/tungo-e2e/guest.py >/dev/console 2>&1 &
echo 'TUNGO_E2E_VM_BOOTED'
while true; do sleep 3600; done
