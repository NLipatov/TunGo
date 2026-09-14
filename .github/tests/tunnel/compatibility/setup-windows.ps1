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
Invoke-WebRequest 'https://www.wintun.net/builds/wintun-0.14.1.zip' -OutFile $archive
Expand-Archive $archive -DestinationPath $unpacked -Force
Copy-Item "$unpacked/wintun/bin/$Architecture/wintun.dll" "$env:SystemRoot/System32/wintun.dll"
