$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('planet-publish-build-test-' + [guid]::NewGuid().ToString('N'))
$source = Join-Path $root 'source'
$build = Join-Path $root 'build'
$target = Join-Path $build 'windows-amd64'
[IO.Directory]::CreateDirectory($source) | Out-Null
[IO.Directory]::CreateDirectory($target) | Out-Null
$files = @('Planet Windows.exe','ipfs.exe','icon.png','kubo-LICENSE.txt','kubo-LICENSE-MIT.txt','kubo-LICENSE-APACHE.txt','LICENSE-PLANET.txt','THIRD-PARTY-NOTICES.txt','KUBO-BUILDINFO.txt','KUBO-DEPENDENCY-LICENSES.txt','planet-windows-safe-setup-0.3.0-amd64.exe')
foreach ($file in $files) { [IO.File]::WriteAllText((Join-Path $source $file), "new $file") }
[IO.File]::WriteAllText((Join-Path $target 'planet-windows-setup-0.1.0-amd64.exe'), 'old installer')
[IO.File]::WriteAllText((Join-Path $target 'personal-file.txt'), 'keep')
$publish = Join-Path $PSScriptRoot 'publish-build.ps1'
& $publish -BundleDir $source -BuildDir $build -InstallerName 'planet-windows-safe-setup-0.3.0-amd64.exe'
foreach ($file in $files) {
    if ([IO.File]::ReadAllText((Join-Path $target $file)) -ne "new $file") { throw "Wrong file: $file" }
}
if (Test-Path -LiteralPath (Join-Path $target 'planet-windows-setup-0.1.0-amd64.exe')) { throw 'Obsolete installer remains in latest output' }
$backup = @(Get-ChildItem -LiteralPath $build -Directory -Filter '.previous-*')
if ($backup.Count -ne 1 -or [IO.File]::ReadAllText((Join-Path $backup[0].FullName 'personal-file.txt')) -ne 'keep') { throw 'Original build was not preserved' }
Remove-Item -LiteralPath (Join-Path $source 'ipfs.exe')
$failed = $false
try { & $publish -BundleDir $source -BuildDir $build -InstallerName 'planet-windows-safe-setup-0.3.0-amd64.exe' } catch { $failed = $true }
if (-not $failed -or [IO.File]::ReadAllText((Join-Path $target 'ipfs.exe')) -ne 'new ipfs.exe') { throw 'Failed publication altered latest output' }
Write-Host 'Build output publishing checks passed.'
