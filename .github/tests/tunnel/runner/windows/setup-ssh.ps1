param([Parameter(Mandatory)][string]$Workdir)
$ErrorActionPreference = 'Stop'
$PSNativeCommandArgumentPassing = 'Standard'

$keyDirectory = Join-Path $Workdir 'tungo-ssh'
# Restrict these newly created files to SYSTEM and Administrators.
& icacls $keyDirectory /reset | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot reset the temporary SSH directory ACL' }
& icacls $keyDirectory /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict the temporary SSH directory' }
foreach ($name in @('identity', 'client')) {
    & "$Workdir/openssh/ssh-keygen.exe" -q -t ed25519 -N '' -f "$keyDirectory/$name"
    if ($LASTEXITCODE -ne 0) { throw "Cannot generate temporary SSH $name" }
}
# ssh-keygen may add a user-specific ACL; the host key must inherit only SY/AG.
& icacls "$keyDirectory/client" /reset | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot restrict the SSH host key ACL' }
& icacls "$keyDirectory/client" /setowner '*S-1-5-32-544' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot set the SSH host key owner' }

$clientUser = [Environment]::UserName.ToLowerInvariant()
@"
ListenAddress 127.0.0.1
Port 2223
HostKey "$keyDirectory/client"
AuthorizedKeysFile "$keyDirectory/identity.pub"
PidFile "$keyDirectory/sshd.pid"
AllowUsers $clientUser
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
AllowTcpForwarding no
AllowAgentForwarding no
PermitTTY no
"@ | Set-Content "$keyDirectory/sshd-client.conf" -Encoding utf8NoBOM

# Encoding protects paths with spaces and quotes through the SSH default shell.
$quotedDirectory = $Workdir.Replace("'", "''")
$script = '& ''{0}/tungo-e2e.exe'' client ''{0}''; exit $LASTEXITCODE' -f $quotedDirectory
$encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($script))
@{
    SSH = '127.0.0.1:2223'
    User = $clientUser
    Command = "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand $encoded"
} | ConvertTo-Json | Set-Content "$Workdir/client.json" -Encoding utf8NoBOM
