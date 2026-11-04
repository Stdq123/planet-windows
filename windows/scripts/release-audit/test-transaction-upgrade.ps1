[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$OldInstaller,
    [Parameter(Mandatory = $true)][string]$NewInstaller,
    [Parameter(Mandatory = $true)][string]$LegacySafeInstaller,
    [Parameter(Mandatory = $true)][string]$LegacyInstaller,
    [string]$Image = 'planet-release-audit:local'
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Utility/Microsoft.PowerShell.Utility.psd1') -Force -ErrorAction Stop
$old = (Resolve-Path -LiteralPath $OldInstaller).Path
$new = (Resolve-Path -LiteralPath $NewInstaller).Path
$legacySafe = (Resolve-Path -LiteralPath $LegacySafeInstaller).Path
$legacy = (Resolve-Path -LiteralPath $LegacyInstaller).Path
$exe = Join-Path ([IO.Path]::GetDirectoryName($new)) 'Planet Windows.exe'
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw 'New installer directory must include the matching standalone Planet Windows.exe.' }
$expected = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant()
$test = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot 'test-transaction-upgrade.sh')).Path
& docker run --rm --network none --read-only --tmpfs /tmp:rw,exec,size=2g `
    --mount "type=bind,source=$old,target=/input/old.exe,readonly" `
    --mount "type=bind,source=$new,target=/input/new.exe,readonly" `
    --mount "type=bind,source=$legacySafe,target=/input/legacy-safe.exe,readonly" `
    --mount "type=bind,source=$legacy,target=/input/legacy.exe,readonly" `
    --mount "type=bind,source=$test,target=/input/test.sh,readonly" `
    --env PLANET_DISPOSABLE_CONTAINER=yes --env "EXPECTED_EXE_SHA256=$expected" $Image sh /input/test.sh
if ($LASTEXITCODE -ne 0) { throw "Isolated transaction-upgrade test failed (exit $LASTEXITCODE)." }
