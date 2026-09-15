#!/usr/bin/env bash
set -euo pipefail

workdir=$1
client_user=$2
keydir="$workdir/tungo-ssh"

chmod 700 "$keydir"
ssh-keygen -q -t ed25519 -N '' -f "$keydir/identity"
ssh-keygen -q -t ed25519 -N '' -f "$keydir/client"
# sshd checks authorized keys as the login user; only the public identity is needed.
chown "$client_user" "$keydir" "$keydir/identity.pub"
use_pam=no
if [[ $(uname -s) == Linux && -f /etc/pam.d/sshd ]]; then
    use_pam=yes
fi
cat >"$keydir/sshd-client.conf" <<EOF
ListenAddress 127.0.0.1
Port 2223
HostKey "$keydir/client"
AuthorizedKeysFile "$keydir/identity.pub"
PidFile "$keydir/sshd.pid"
AllowUsers $client_user
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
UsePAM $use_pam
AllowTcpForwarding no
AllowAgentForwarding no
PermitTTY no
PrintMotd no
EOF
if [[ $(uname -s) == Linux ]]; then
    mkdir -p /run/sshd
fi

# The SSH command launches the client part of the harness with native privileges.
printf -v client_command 'sudo -n %q client %q' \
    "$workdir/tungo-e2e" "$workdir"
jq -n --arg user "$client_user" --arg command "$client_command" \
    '{SSH:"127.0.0.1:2223", User:$user, Command:$command}' >"$workdir/client.json"
