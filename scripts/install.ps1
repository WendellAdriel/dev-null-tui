param(
    [string]$Version = 'latest',
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\devnull')
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$repo = 'WendellAdriel/dev-null-tui'
if ($env:OS -ne 'Windows_NT') { throw 'Use install.sh on Linux, macOS, or WSL.' }
$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
$arch = switch ($architecture) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { throw "Unsupported Windows architecture: $architecture" }
}
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
if ($Version -eq 'latest') {
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -TimeoutSec 180
    if ($latest.draft -or $latest.prerelease) { throw 'Expected a stable release.' }
    $Version = $latest.tag_name
}
if ($Version -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\z') {
    throw 'Expected a stable version such as v0.1.0.'
}

$asset = "devnull_windows_$arch.exe"
$base = "https://github.com/$repo/releases/download/$Version"
$null = New-Item -ItemType Directory -Force -Path $InstallDir
$InstallDir = (Resolve-Path -LiteralPath $InstallDir).Path
$destination = Join-Path $InstallDir 'devnull.exe'
$backup = "$destination.old"
$temp = Join-Path $InstallDir ('.devnull-install-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $temp
$binary = Join-Path $temp 'binary'
$checksums = Join-Path $temp 'checksums.txt'
try {
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $checksums -TimeoutSec 180
    $matches = @(Get-Content -LiteralPath $checksums | Where-Object { $_ -cmatch ('^[0-9a-fA-F]{64}  ' + [regex]::Escape($asset) + '$') })
    if ($matches.Count -ne 1) { throw 'The release checksum is missing, invalid, or duplicated.' }
    $expected = $matches[0].Substring(0, 64)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $binary -TimeoutSec 180
    $length = (Get-Item -LiteralPath $binary).Length
    if ($length -eq 0 -or $length -gt 134217728) { throw 'Invalid release binary size.' }
    if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash -ine $expected) {
        throw 'Checksum verification failed. The installed binary was not changed.'
    }
    if (Test-Path -LiteralPath $backup) { Remove-Item -LiteralPath $backup }
    $hadBinary = Test-Path -LiteralPath $destination
    if ($hadBinary) {
        if ((Get-Item -LiteralPath $destination).PSIsContainer) { throw 'The installation destination is a directory.' }
        Move-Item -LiteralPath $destination -Destination $backup
    }
    try {
        Move-Item -LiteralPath $binary -Destination $destination
    } catch {
        if ($hadBinary) { Move-Item -LiteralPath $backup -Destination $destination }
        throw
    }
    Write-Output "Installed devnull $Version to $destination"
    Write-Output "Add $InstallDir to your user PATH in Windows Environment Variables, then open a new terminal."
} finally {
    if (Test-Path -LiteralPath $binary) { Remove-Item -LiteralPath $binary }
    if (Test-Path -LiteralPath $checksums) { Remove-Item -LiteralPath $checksums }
    Remove-Item -LiteralPath $temp
}
