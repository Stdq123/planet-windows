param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [Parameter(Mandatory = $true)][string]$BuildDir,
    [Parameter(Mandatory = $true)][string]$InstallerName
)
$ErrorActionPreference = 'Stop'
foreach ($taskModule in @('Utility', 'Management', 'Security')) {
    Import-Module (Join-Path $PSHOME "Modules/Microsoft.PowerShell.$taskModule/Microsoft.PowerShell.$taskModule.psd1") -Force -ErrorAction Stop
}
if ([IO.Path]::GetFileName($InstallerName) -cne $InstallerName -or $InstallerName -cnotmatch '^planet-windows-safe-setup-([0-9A-Za-z.-]+)-amd64\.exe$') { throw 'Invalid installer name.' }
$taskVersion = $Matches[1]
$taskParsedVersion = & bun (Join-Path $PSScriptRoot '../../scripts/release-version.ts') $taskVersion
if ($LASTEXITCODE -ne 0) { throw 'Invalid installer name.' }
if (($taskParsedVersion | ConvertFrom-Json).version -cne $taskVersion) { throw 'Invalid installer name.' }
$source = (Resolve-Path -LiteralPath $BundleDir).Path
if (-not (Test-Path -LiteralPath $source -PathType Container)) { throw 'Bundle directory not found.' }
[IO.Directory]::CreateDirectory($BuildDir) | Out-Null
$root = (Resolve-Path -LiteralPath $BuildDir).Path
foreach ($path in @($source, $root)) {
    if ((Get-Item -LiteralPath $path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Directory must not be a link: $path" }
}
$target = Join-Path $root 'windows-amd64'
if ([string]::Equals($source, $target, [StringComparison]::OrdinalIgnoreCase)) { throw 'Bundle and destination must be different.' }
$files = @('Planet Windows.exe','ipfs.exe','icon.png','kubo-LICENSE.txt','kubo-LICENSE-MIT.txt','kubo-LICENSE-APACHE.txt','LICENSE-PLANET.txt','THIRD-PARTY-NOTICES.txt','KUBO-BUILDINFO.txt','KUBO-DEPENDENCY-LICENSES.txt',$InstallerName)
foreach ($name in $files) {
    $file = Join-Path $source $name
    if (-not (Test-Path -LiteralPath $file -PathType Leaf) -or ((Get-Item -LiteralPath $file -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Release file missing or invalid: $name" }
}
$lock = [IO.File]::Open((Join-Path $root '.planet-publish.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
try {
    if (Test-Path -LiteralPath $target) {
        $existing = Get-Item -LiteralPath $target -Force
        if (-not $existing.PSIsContainer -or ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Existing build destination must be a normal directory.' }
        $running = @(Get-Process -Name 'Planet Windows','ipfs' -ErrorAction SilentlyContinue | Where-Object {
            try { $_.Path -and [string]::Equals([IO.Path]::GetDirectoryName($_.Path), $target, [StringComparison]::OrdinalIgnoreCase) } catch { $false }
        })
        if ($running.Count) { throw "Close Planet Windows and its Kubo process running from $target before updating this directory." }
    }
    $id = [guid]::NewGuid().ToString('N')
    $stage = Join-Path $root ('.latest-' + $id)
    $previous = Join-Path $root ('.previous-' + $id)
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    foreach ($name in $files) {
        $from = Join-Path $source $name
        $to = Join-Path $stage $name
        Copy-Item -LiteralPath $from -Destination $to
        if ((Get-FileHash -Algorithm SHA256 -LiteralPath $from).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $to).Hash) { throw "Copy verification failed: $name" }
    }
    $moved = $false
    try {
        if (Test-Path -LiteralPath $target) { Move-Item -LiteralPath $target -Destination $previous; $moved = $true }
        Move-Item -LiteralPath $stage -Destination $target
    } catch {
        if ($moved -and -not (Test-Path -LiteralPath $target)) {
            try { Move-Item -LiteralPath $previous -Destination $target } catch { throw "Could not restore previous build at $previous. $($_.Exception.Message)" }
        }
        throw
    }
    if ($moved) { Write-Host "Previous build preserved: $previous" }
    Write-Host "Latest build directory: $target"
} finally { $lock.Dispose() }
