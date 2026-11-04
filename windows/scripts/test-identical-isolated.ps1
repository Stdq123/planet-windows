[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$OldInstaller,
    [Parameter(Mandatory = $true)][string]$NewInstaller,
    [Parameter(Mandatory = $true)][string]$DifferentInstaller,
    [string]$Image = 'planet-release-audit:local'
)
$ErrorActionPreference = 'Stop'
$old = (Resolve-Path -LiteralPath $OldInstaller).Path
$new = (Resolve-Path -LiteralPath $NewInstaller).Path
$different = (Resolve-Path -LiteralPath $DifferentInstaller).Path
$test = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot 'release-audit/test-identical-isolated.sh')).Path
& docker run --rm --network none --read-only --tmpfs /tmp:rw,exec,size=2g `
    --mount "type=bind,source=$old,target=/input/old.exe,readonly" `
    --mount "type=bind,source=$new,target=/input/new.exe,readonly" `
    --mount "type=bind,source=$different,target=/input/different.exe,readonly" `
    --mount "type=bind,source=$test,target=/input/test.sh,readonly" `
    --env PLANET_DISPOSABLE_CONTAINER=yes $Image sh /input/test.sh
if ($LASTEXITCODE -ne 0) { throw "Isolated identical-install test failed (exit $LASTEXITCODE)." }
