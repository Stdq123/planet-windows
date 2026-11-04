[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$OldInstaller,
    # Test fixture must contain PLANET-UPGRADE-FIXTURE-V2 in KUBO-BUILDINFO.txt.
    # A production release is never used as the synthetic candidate.
    [Parameter(Mandatory = $true)][string]$NewInstaller,
    [string]$Image = 'planet-release-audit:local'
)
$ErrorActionPreference = 'Stop'
$oldPath = (Resolve-Path -LiteralPath $OldInstaller).Path
$newPath = (Resolve-Path -LiteralPath $NewInstaller).Path
$testPath = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot 'release-audit/test-upgrade-isolated.sh')).Path
$project = [IO.Directory]::GetParent($PSScriptRoot).FullName
$scratch = Join-Path $project ('.mygo/release-audit-tests/preflight-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($scratch) | Out-Null
$guard = Join-Path $scratch 'guard.exe'
$holder = Join-Path $scratch 'holder.exe'
$names = @('Planet Windows.exe', 'ipfs.exe', 'icon.png', 'kubo-LICENSE.txt', 'kubo-LICENSE-MIT.txt', 'kubo-LICENSE-APACHE.txt', 'LICENSE-PLANET.txt', 'THIRD-PARTY-NOTICES.txt', 'KUBO-BUILDINFO.txt', 'KUBO-DEPENDENCY-LICENSES.txt')
$fixtureManifest = [ordered]@{}
foreach ($name in $names) { $fixtureManifest[$name] = '0' * 64 }
$json = ConvertTo-Json -Compress -InputObject $fixtureManifest
$encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json))
Push-Location $project
try {
    & go build -ldflags "-X main.manifestBase64=$encoded" -o $guard ./scripts/release-audit/installer-guard
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build test guard.' }
    & go build -o $holder ./scripts/release-audit/lock-holder
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build lock holder.' }
} finally { Pop-Location }
& docker image inspect $Image --format '{{.Id}}' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Isolated test image is unavailable.' }
& docker run --rm --network none --read-only --tmpfs /tmp:rw,exec,size=2g `
    --mount "type=bind,source=$oldPath,target=/input/old.exe,readonly" `
    --mount "type=bind,source=$newPath,target=/input/new.exe,readonly" `
    --mount "type=bind,source=$guard,target=/input/guard.exe,readonly" `
    --mount "type=bind,source=$holder,target=/input/holder.exe,readonly" `
    --mount "type=bind,source=$testPath,target=/input/test.sh,readonly" `
    --env PLANET_DISPOSABLE_CONTAINER=yes $Image sh /input/test.sh
if ($LASTEXITCODE -ne 0) { throw "Isolated upgrade test failed (exit $LASTEXITCODE)." }
