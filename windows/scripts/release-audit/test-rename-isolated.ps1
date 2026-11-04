[CmdletBinding()]
param([string]$Image = 'planet-release-audit:local')
$ErrorActionPreference = 'Stop'
$project = [IO.Directory]::GetParent([IO.Directory]::GetParent($PSScriptRoot).FullName).FullName
$scratch = Join-Path $project ('.mygo/release-audit-tests/rename-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($scratch) | Out-Null
$probe = Join-Path $scratch 'renameprobe.exe'
Push-Location $project
try {
    & go test -c -o $probe ./scripts/release-audit/renameprobe
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build rename probe.' }
} finally { Pop-Location }
$test = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot 'test-rename-isolated.sh')).Path
& docker run --rm --network none --read-only --tmpfs /tmp:rw,exec,size=2g `
    --mount "type=bind,source=$probe,target=/input/renameprobe.exe,readonly" `
    --mount "type=bind,source=$test,target=/input/test.sh,readonly" `
    --env PLANET_DISPOSABLE_CONTAINER=yes $Image sh /input/test.sh
if ($LASTEXITCODE -ne 0) { throw "Isolated rename probe failed (exit $LASTEXITCODE)." }
