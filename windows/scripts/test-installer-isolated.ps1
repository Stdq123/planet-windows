[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [string]$PreviousInstaller,
    [switch]$SafeChecks,
    [string]$Image = 'planet-release-audit:local'
)
$ErrorActionPreference = 'Stop'
$installerPath = (Resolve-Path -LiteralPath $Installer).Path
if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) { throw 'Installer must be a file.' }
$testName = if ($SafeChecks) { 'release-audit/test-safe-installer.sh' } else { 'release-audit/test-installer.sh' }
$testScript = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot $testName)).Path
$holderPath = $null
if ($SafeChecks) {
    $projectRoot = [System.IO.Directory]::GetParent($PSScriptRoot).FullName
    $holderDir = Join-Path $projectRoot ('.mygo/release-audit-tests/holder-' + [Guid]::NewGuid().ToString('N'))
    [System.IO.Directory]::CreateDirectory($holderDir) | Out-Null
    $holderPath = Join-Path $holderDir 'lock-holder.exe'
    Push-Location $projectRoot
    try { & go build -o $holderPath ./scripts/release-audit/lock-holder; if ($LASTEXITCODE -ne 0) { throw 'Failed to build test lock holder.' } }
    finally { Pop-Location }
}
& docker image inspect $Image --format '{{.Id}}' | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Docker image $Image is unavailable. Build scripts/release-audit/Dockerfile first." }
# No writable host mounts, no Docker socket, and no network. Only the setup
# and test script are visible via read-only binds; Wine state lives in tmpfs.
$dockerArgs = @('run', '--rm', '--network', 'none', '--read-only', '--tmpfs', '/tmp:rw,exec,size=2g',
    '--mount', "type=bind,source=$installerPath,target=/input/setup.exe,readonly",
    '--mount', "type=bind,source=$testScript,target=/input/test-installer.sh,readonly")
if ($PreviousInstaller) {
    $previousPath = (Resolve-Path -LiteralPath $PreviousInstaller).Path
    if (-not (Test-Path -LiteralPath $previousPath -PathType Leaf)) { throw 'PreviousInstaller must be a file.' }
    $dockerArgs += @('--mount', "type=bind,source=$previousPath,target=/input/previous.exe,readonly")
}
if ($holderPath) { $dockerArgs += @('--mount', "type=bind,source=$holderPath,target=/input/holder.exe,readonly") }
$dockerArgs += @('--env', 'PLANET_DISPOSABLE_CONTAINER=yes', $Image, 'sh', '/input/test-installer.sh')
& docker @dockerArgs
if ($LASTEXITCODE -ne 0) { throw "Isolated installer test failed (exit $LASTEXITCODE)." }
