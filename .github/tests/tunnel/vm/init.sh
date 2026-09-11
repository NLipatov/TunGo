#!/bin/sh
set -eu
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mkdir -p /dev/net /run/netns
mount -t tmpfs tmpfs /run
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
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv6.conf.all.forwarding=1
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
ip netns exec target python3 /opt/tungo-e2e/http_server.py >/tmp/target.log 2>&1 &
ip netns exec target python3 /opt/tungo-e2e/http_server.py --bind fd73:7467:6f::2 >/tmp/target6.log 2>&1 &
python3 /opt/tungo-e2e/guest.py >/dev/console 2>&1 &
# Only VPN UDP metadata is logged; control traffic carries generated keys.
tcpdump -i eth0 -nn -l 'udp port 9090' >/tmp/udp.log 2>&1 &
echo 'TUNGO_E2E_VM_BOOTED'
while true; do sleep 3600; done
