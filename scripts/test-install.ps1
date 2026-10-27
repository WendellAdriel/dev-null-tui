param([string]$WorkDir, [string]$Scenario)
$ErrorActionPreference = 'Stop'
$env:LOCALAPPDATA = Join-Path $WorkDir 'local app data'
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\devnull'
$null = New-Item -ItemType Directory -Force -Path $installDir
$destination = Join-Path $installDir 'devnull.exe'
[IO.File]::WriteAllText($destination, 'old executable')
$fixture = Join-Path $WorkDir 'fixture'
[IO.File]::WriteAllText($fixture, 'new executable')
$digest = (Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash
if ($Scenario -eq 'checksum mismatch') { $digest = '0' * 64 }
$manifest = "$digest  devnull_windows_amd64.exe`n$digest  devnull_windows_arm64.exe`n"
if ($Scenario -eq 'duplicate checksum') { $manifest += $manifest }
function Invoke-RestMethod { return @{ tag_name = 'v0.1.0'; draft = $false; prerelease = $false } }
function Invoke-WebRequest {
    param($Uri, $OutFile, $TimeoutSec, [switch]$UseBasicParsing)
    if ($Uri.EndsWith('/checksums.txt')) { [IO.File]::WriteAllText($OutFile, $manifest); return }
    if ($Uri -notmatch '/devnull_windows_(amd64|arm64)\.exe$') { throw "Unexpected request: $Uri" }
    if ($Scenario -eq 'download failure') { throw 'Fixture download failed' }
    Copy-Item -LiteralPath $fixture -Destination $OutFile
}
$failed = $false
try { Get-Content -Raw -LiteralPath "$PSScriptRoot/install.ps1" | Invoke-Expression } catch { $failed = $true }
$got = [IO.File]::ReadAllText($destination)
if ($Scenario -eq 'success') {
    if ($failed -or $got -ne 'new executable') { throw 'Expected successful installation' }
} elseif (-not $failed -or $got -ne 'old executable') { throw 'Failed installation changed the executable' }
if (@(Get-ChildItem -LiteralPath $installDir -Force | Where-Object { $_.Name.StartsWith('.devnull-install-') }).Count -ne 0) {
    throw 'Installer left temporary downloads'
}
