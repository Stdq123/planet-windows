[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$PayloadDir)
$ErrorActionPreference = 'Stop'
$source = (Resolve-Path -LiteralPath $PayloadDir).Path
$project = [IO.Directory]::GetParent([IO.Directory]::GetParent($PSScriptRoot).FullName).FullName
$root = Join-Path $project ('.mygo/release-audit-tests/upgrade-fixture-' + [guid]::NewGuid().ToString('N'))
$target = Join-Path $root 'payload'
[IO.Directory]::CreateDirectory($target) | Out-Null
$names = @('Planet Windows.exe', 'ipfs.exe', 'icon.png', 'kubo-LICENSE.txt', 'kubo-LICENSE-MIT.txt', 'kubo-LICENSE-APACHE.txt', 'LICENSE-PLANET.txt', 'THIRD-PARTY-NOTICES.txt', 'KUBO-BUILDINFO.txt', 'KUBO-DEPENDENCY-LICENSES.txt')
foreach ($name in $names) {
    $file = Join-Path $source $name
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Fixture payload missing: $name" }
    Copy-Item -LiteralPath $file -Destination (Join-Path $target $name)
}
[IO.File]::AppendAllText((Join-Path $target 'KUBO-BUILDINFO.txt'), "`r`nPLANET-UPGRADE-FIXTURE-V2`r`n", [Text.UTF8Encoding]::new($false))
# Test-only PE overlay makes the executable hash differ as well, so acceptance
# proves the actual exe file was switched. This does not emulate new app logic.
$fixtureExe = Join-Path $target 'Planet Windows.exe'
$fixtureStream = [IO.File]::Open($fixtureExe, [IO.FileMode]::Append, [IO.FileAccess]::Write, [IO.FileShare]::None)
try { $fixtureMarker = [Text.Encoding]::ASCII.GetBytes("`r`nPLANET-TEST-ONLY-EXE-V03`r`n"); $fixtureStream.Write($fixtureMarker, 0, $fixtureMarker.Length); $fixtureStream.Flush($true) } finally { $fixtureStream.Dispose() }
$output = Join-Path $target 'TEST-ONLY-upgrade-setup.exe'
& (Join-Path $project 'scripts/build-safe-installer.ps1') -PayloadDir $target -OutputFile $output -ReleaseVersion '0.3.0'
if (-not (Test-Path -LiteralPath $output -PathType Leaf)) { throw 'Fixture build failed.' }
Write-Host "Test-only installer (do not distribute): $output"
