#!/bin/sh
set -eu
export LC_ALL=C
echo interfaces
ls /sys/class/net
for family in 4 6; do
    echo "routes$family"
    routes=$(ip -"$family" route show table main)
    printf '%s\n' "$routes" | sort
    echo "firewall$family"
    firewall=iptables
    if [ "$family" = 6 ]; then firewall=ip6tables; fi
    for table in filter nat mangle; do
        "$firewall" -t "$table" -S
    done
done
