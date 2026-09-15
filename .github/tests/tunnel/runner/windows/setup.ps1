param([ValidateSet('amd64', 'arm64')][string]$Architecture)
$ErrorActionPreference = 'Stop'

# Use native QEMU binaries on both Windows runner architectures.
$release = '20260811'
$directory = if ($Architecture -eq 'arm64') { 'aarch64' } else { 'w64' }
$prefix = if ($Architecture -eq 'arm64') { 'qemu-arm' } else { 'qemu-w64' }
$base = "https://qemu.weilnetz.de/$directory/$prefix-setup-$release"
$installer = Join-Path $env:RUNNER_TEMP 'qemu-setup.exe'
Invoke-WebRequest "$base.exe" -OutFile $installer
$checksumFile = Join-Path $env:RUNNER_TEMP 'qemu.sha512'
Invoke-WebRequest "$base.sha512" -OutFile $checksumFile
$checksum = (Get-Content $checksumFile -Raw).Trim().Split()[0]
if ((Get-FileHash $installer -Algorithm SHA512).Hash -ne $checksum) {
    throw 'QEMU installer checksum mismatch'
}
$qemuDirectory = Join-Path $env:RUNNER_TEMP 'qemu'
$process = Start-Process -FilePath $installer -ArgumentList @('/S', "/D=$qemuDirectory") -Wait -PassThru
if ($process.ExitCode -ne 0) { throw "QEMU installation failed: $($process.ExitCode)" }
if (!(Test-Path "$qemuDirectory/qemu-system-x86_64.exe") -or !(Test-Path "$qemuDirectory/qemu-system-aarch64.exe")) {
    throw 'QEMU installation is missing a required guest architecture'
}
$qemuDirectory | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append

# The same signed driver distribution used by the project's Windows releases.
$archive = Join-Path $env:RUNNER_TEMP 'wintun.zip'
$unpacked = Join-Path $env:RUNNER_TEMP 'wintun'
$wintunChecksum = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51'
Invoke-WebRequest 'https://www.wintun.net/builds/wintun-0.14.1.zip' -OutFile $archive
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $wintunChecksum) {
    throw 'Wintun archive checksum mismatch'
}
Expand-Archive $archive -DestinationPath $unpacked -Force
Copy-Item "$unpacked/wintun/bin/$Architecture/wintun.dll" "$env:SystemRoot/System32/wintun.dll"

# Use one native, pinned OpenSSH distribution for the disposable localhost server.
$sshRelease = '10.0.0.0p2-Preview'
$sshPlatform = if ($Architecture -eq 'arm64') { 'ARM64' } else { 'Win64' }
$sshChecksum = if ($Architecture -eq 'arm64') {
    '698c6aec31c1dd0fb996206e8741f4531a97355686b5431ef347d531b07fcd42'
} else {
    '23f50f3458c4c5d0b12217c6a5ddfde0137210a30fa870e98b29827f7b43aba5'
}
$sshArchive = Join-Path $env:RUNNER_TEMP 'openssh.zip'
Invoke-WebRequest "https://github.com/PowerShell/Win32-OpenSSH/releases/download/$sshRelease/OpenSSH-$sshPlatform.zip" -OutFile $sshArchive
if ((Get-FileHash $sshArchive -Algorithm SHA256).Hash -ne $sshChecksum) {
    throw 'OpenSSH archive checksum mismatch'
}
$sshUnpacked = Join-Path $env:RUNNER_TEMP 'openssh-unpacked'
Expand-Archive $sshArchive -DestinationPath $sshUnpacked
Move-Item "$sshUnpacked/OpenSSH-$sshPlatform" "$env:RUNNER_TEMP/openssh"
