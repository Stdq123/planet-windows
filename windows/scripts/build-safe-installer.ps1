[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PayloadDir,
    [Parameter(Mandatory = $true)][string]$OutputFile,
    [string]$ReleaseVersion = '',
    [string]$Makensis = (Join-Path $env:LOCALAPPDATA 'mygo/nsis-3.13/makensis.exe')
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Utility/Microsoft.PowerShell.Utility.psd1') -Force -ErrorAction Stop
$projectRoot = [System.IO.Directory]::GetParent($PSScriptRoot).FullName
if (-not $ReleaseVersion) { $ReleaseVersion = ([IO.File]::ReadAllText((Join-Path $projectRoot 'release.json')) | ConvertFrom-Json).version }
if ($ReleaseVersion -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid release version.' }
$payloadPath = (Resolve-Path -LiteralPath $PayloadDir).Path
$outputPath = [System.IO.Path]::GetFullPath($OutputFile)
if (Test-Path -LiteralPath $outputPath) { throw "Refusing to overwrite existing file: $outputPath" }
if ($payloadPath -match '[`$"\r\n]' -or $outputPath -match '[`$"\r\n]' -or $projectRoot -match '[`$"\r\n]') { throw 'NSIS paths containing $, quotes, or line breaks are not supported.' }
if (-not (Test-Path -LiteralPath ([System.IO.Path]::GetDirectoryName($outputPath)) -PathType Container)) { throw 'Installer output parent directory does not exist.' }
if (-not (Test-Path -LiteralPath $Makensis -PathType Leaf)) { throw 'NSIS 3.13 compiler not found.' }
$names = @('Planet Windows.exe', 'ipfs.exe', 'icon.png', 'kubo-LICENSE.txt', 'kubo-LICENSE-MIT.txt', 'kubo-LICENSE-APACHE.txt', 'LICENSE-PLANET.txt', 'THIRD-PARTY-NOTICES.txt', 'KUBO-BUILDINFO.txt', 'KUBO-DEPENDENCY-LICENSES.txt')
$manifest = [ordered]@{}
foreach ($name in $names) {
    $file = Join-Path $payloadPath $name
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Payload missing: $name" }
    $manifest[$name] = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
}
$json = ConvertTo-Json -Compress -InputObject $manifest
$encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json))
$scratch = Join-Path $projectRoot ('.mygo/release-audit-tests/guard-' + [Guid]::NewGuid().ToString('N'))
[System.IO.Directory]::CreateDirectory($scratch) | Out-Null
$guard = Join-Path $scratch 'installer-guard.exe'
$previous = @{}
foreach ($key in @('GOOS','GOARCH','CGO_ENABLED')) { $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process') }
try {
    $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    Push-Location $projectRoot
    try {
        & go build -trimpath -ldflags "-s -w -X main.manifestBase64=$encoded -X main.releaseVersion=$ReleaseVersion" -o $guard ./scripts/release-audit/installer-guard
        if ($LASTEXITCODE -ne 0) { throw 'Failed to compile the installer guard.' }
    } finally { Pop-Location }
} finally {
    foreach ($key in $previous.Keys) { [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process') }
}
$script = Join-Path $PSScriptRoot 'release-audit/safe-installer.nsi'
& $Makensis "/DOUTPUT=$outputPath" "/DPAYLOAD=$payloadPath" "/DGUARD=$guard" "/DVERSION=$ReleaseVersion" $script
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $outputPath -PathType Leaf)) { throw 'Independent NSIS installer generation failed.' }
foreach ($name in $names) {
    $current = (Get-FileHash -LiteralPath (Join-Path $payloadPath $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($current -ne $manifest[$name]) { throw "Payload changed during installer creation: $name. Do not distribute this candidate." }
}
Write-Host "Candidate installer created: $outputPath"
