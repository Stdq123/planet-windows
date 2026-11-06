# Export source locally. No installation, Git mutation, tagging or publishing.
[CmdletBinding()]
param([string]$RepositoryRoot = (Split-Path -Parent $PSScriptRoot), [string]$OutputDirectory = '')

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
foreach ($taskModule in @('Utility', 'Management', 'Security')) {
    Import-Module (Join-Path $PSHOME "Modules/Microsoft.PowerShell.$taskModule/Microsoft.PowerShell.$taskModule.psd1") -Force -ErrorAction Stop
}
Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
$taskUtf8 = [Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = $taskUtf8
$OutputEncoding = $taskUtf8
$taskRoot = (Resolve-Path -LiteralPath $RepositoryRoot).Path

function Invoke-SourceGit([string[]]$Arguments) {
    $result = @(& git -C $taskRoot @Arguments)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read source Git checkout.' }
    return $result
}
function Get-SourceHash([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}
function Assert-CleanSource {
    if (@(Invoke-SourceGit @('status', '--porcelain=v1', '--untracked-files=all')).Count -gt 0) {
        throw 'Source preparation requires a clean checkout.'
    }
}
function Assert-SafeTrackedPath([string]$Path) {
    if ($Path -match '[\\\x00-\x1f]' -or $Path -match '(^|/)(\.git|node_modules|\.mygo|\.env[^/]*)(/|$)' -or
        $Path -match '^(build|dist)/|^windows/(build|dist|vendor|resources/windows-amd64)/' -or
        $Path -match '\.(exe|dll|syso|pem|key|pfx|p12)$') {
        throw "Unsafe tracked source path: $Path"
    }
}

$taskGitRoot = (Invoke-SourceGit @('rev-parse', '--show-toplevel')) -join ''
if ([IO.Path]::GetFullPath($taskGitRoot) -ine $taskRoot) { throw 'RepositoryRoot must be the Git top-level directory.' }
Assert-CleanSource
$taskCommit = (Invoke-SourceGit @('rev-parse', 'HEAD')) -join ''
$taskTree = (Invoke-SourceGit @('rev-parse', 'HEAD^{tree}')) -join ''
$taskEntries = ((Invoke-SourceGit @('ls-tree', '-r', '-z', '--format=%(objectmode)%x09%(path)', $taskCommit)) -join "`n").Split([char]0)
$taskPaths = @()
foreach ($taskEntry in $taskEntries) {
    if (-not $taskEntry) { continue }
    $taskFields = $taskEntry.Split([char]9, 2)
    if ($taskFields.Count -ne 2 -or $taskFields[0] -notin @('100644', '100755')) { throw 'Source archive requires regular tracked files; links and submodules need separate review.' }
    Assert-SafeTrackedPath $taskFields[1]
    $taskPaths += $taskFields[1]
}
foreach ($taskRequired in @('LICENSE', 'scripts/release-version.ts', 'windows/release.json', 'windows/go.mod', 'windows/go.sum', 'windows/bun.lock', 'docs/rebuilding-release.md')) {
    if ($taskRequired -cnotin $taskPaths) { throw "Required source input is not tracked: $taskRequired" }
}
$taskVersionJson = (Invoke-SourceGit @('show', ($taskCommit + ':windows/release.json'))) -join "`n"
$taskVersion = ($taskVersionJson | ConvertFrom-Json).version
$taskParsedVersion = & bun (Join-Path $taskRoot 'scripts/release-version.ts') $taskVersion
if ($LASTEXITCODE -ne 0 -or ($taskParsedVersion | ConvertFrom-Json).version -cne $taskVersion) { throw 'Invalid source version.' }
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskRoot ('windows/.mygo/source-releases/' + $taskVersion + '-' + [guid]::NewGuid().ToString('N')) }
$taskOutput = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $taskOutput) { throw 'Source output directory must not already exist.' }
[IO.Directory]::CreateDirectory($taskOutput) | Out-Null
$taskStage = Join-Path $taskOutput 'source'
$taskGitArchive = Join-Path $taskOutput 'tracked-source.zip'
Invoke-SourceGit @('-c', 'core.autocrlf=false', '-c', 'core.eol=lf', 'archive', '--format=zip', ('--output=' + $taskGitArchive), $taskCommit) | Out-Null
[IO.Compression.ZipFile]::ExtractToDirectory($taskGitArchive, $taskStage)
foreach ($taskPath in $taskPaths) {
    if (-not (Test-Path -LiteralPath (Join-Path $taskStage $taskPath) -PathType Leaf)) {
        throw "Tracked source excluded by archive attributes: $taskPath"
    }
}

$taskApp = Join-Path $taskStage 'windows'
$taskLocks = @{}
foreach ($taskLock in @('go.mod', 'go.sum', 'bun.lock')) { $taskLocks[$taskLock] = Get-SourceHash (Join-Path $taskApp $taskLock) }
$taskPreviousEnvironment = @{}
foreach ($taskName in @('GOFLAGS', 'GOWORK', 'GOOS', 'GOARCH', 'CGO_ENABLED')) {
    $taskPreviousEnvironment[$taskName] = [Environment]::GetEnvironmentVariable($taskName, 'Process')
}
try {
    $env:GOFLAGS = '-mod=readonly'; $env:GOWORK = 'off'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Push-Location $taskApp
    try {
        $taskGoModJson = & go mod edit -json
        if ($LASTEXITCODE -ne 0) { throw 'Cannot read locked Go module metadata.' }
        $taskGoMod = ($taskGoModJson -join "`n") | ConvertFrom-Json
        if (@($taskGoMod.Replace).Count -gt 0 -and $null -ne $taskGoMod.Replace) { throw 'Source packaging does not permit module replacements; review and pin them first.' }
        $taskDownloadJson = & go mod download -json github.com/ethereum/go-ethereum
        if ($LASTEXITCODE -ne 0) { throw 'Cannot fetch locked go-ethereum source.' }
        $taskLibrary = ($taskDownloadJson -join "`n") | ConvertFrom-Json
        if ($taskLibrary.Path -cne 'github.com/ethereum/go-ethereum' -or -not $taskLibrary.Sum -or -not $taskLibrary.Dir) { throw 'Incomplete go-ethereum module provenance.' }
        $taskIdentity = "$($taskLibrary.Path) $($taskLibrary.Version) $($taskLibrary.Sum)"
        if ($taskIdentity -cnotin [IO.File]::ReadAllLines((Join-Path $taskApp 'go.sum'))) { throw 'go-ethereum source does not match the committed lock file.' }
        & go mod verify | Write-Host
        if ($LASTEXITCODE -ne 0) { throw 'Go source verification failed.' }
        & go mod vendor
        if ($LASTEXITCODE -ne 0) { throw 'Cannot vendor locked Go source.' }
        # go mod vendor omits dependency tests and unused directories. Preserve
        # the complete go-ethereum module source as well, including its licenses.
        $taskLibraryRoot = (Resolve-Path -LiteralPath $taskLibrary.Dir).Path
        $taskLibraryTarget = Join-Path $taskApp 'vendor/github.com/ethereum/go-ethereum'
        $taskOmissions = @()
        foreach ($taskFile in Get-ChildItem -LiteralPath $taskLibraryRoot -Recurse -File -Force) {
            if ($taskFile.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Linked dependency source is not permitted.' }
            $taskRelative = $taskFile.FullName.Substring($taskLibraryRoot.Length + 1).Replace('\', '/')
            if ($taskRelative -match '\.(exe|dll|syso)$') { $taskOmissions += $taskRelative; continue }
            $taskDestination = Join-Path $taskLibraryTarget $taskRelative
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($taskDestination)) | Out-Null
            Copy-Item -LiteralPath $taskFile.FullName -Destination $taskDestination -Force
            [IO.File]::SetAttributes($taskDestination, [IO.FileAttributes]::Normal)
        }
        foreach ($taskLicense in @('COPYING', 'COPYING.LESSER')) {
            if (-not (Test-Path -LiteralPath (Join-Path $taskLibraryTarget $taskLicense) -PathType Leaf)) { throw "go-ethereum license is missing: $taskLicense" }
        }
        & go mod verify | Write-Host
        if ($LASTEXITCODE -ne 0) { throw 'Go source verification failed after copying.' }
        $taskGoVersion = (& go version) -join ''
        if ($LASTEXITCODE -ne 0) { throw 'Cannot identify Go source tooling.' }
    } finally { Pop-Location }
} finally {
    foreach ($taskName in $taskPreviousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($taskName, $taskPreviousEnvironment[$taskName], 'Process') }
}
foreach ($taskLock in $taskLocks.Keys) {
    if ((Get-SourceHash (Join-Path $taskApp $taskLock)) -cne $taskLocks[$taskLock]) { throw "Source preparation changed a lock file: $taskLock" }
}
Assert-CleanSource
if (((Invoke-SourceGit @('rev-parse', 'HEAD')) -join '') -cne $taskCommit) { throw 'Source HEAD changed during preparation.' }

$taskProvenance = [ordered]@{
    schemaVersion = 1
    sourceCommit = $taskCommit
    appTree = $taskTree
    version = $taskVersion
    goEthereum = [ordered]@{ path = $taskLibrary.Path; version = $taskLibrary.Version; sum = $taskLibrary.Sum }
    lockSha256 = $taskLocks
    vendorModulesSha256 = Get-SourceHash (Join-Path $taskApp 'vendor/modules.txt')
    goTool = $taskGoVersion
    omittedUpstreamBinaries = $taskOmissions
    scope = 'Tracked application source, locked Go vendor source and full go-ethereum source excluding upstream executables. Bun dependencies are locked but downloaded when rebuilding. No publisher credentials or runtime bundles are included.'
    note = 'Source packaging is engineering evidence, not a legal clearance or proof that a distributed binary was built from this archive.'
}
[IO.File]::WriteAllText((Join-Path $taskStage 'SOURCE-PROVENANCE.json'), ($taskProvenance | ConvertTo-Json -Depth 6) + "`n", $taskUtf8)
$taskArchiveName = 'planet-windows-source-' + $taskVersion + '-' + $taskCommit.Substring(0, 12) + '.zip'
$taskArchivePath = Join-Path $taskOutput $taskArchiveName
$taskZip = [IO.Compression.ZipFile]::Open($taskArchivePath, [IO.Compression.ZipArchiveMode]::Create)
try {
    foreach ($taskFile in Get-ChildItem -LiteralPath $taskStage -Recurse -File -Force | Sort-Object FullName) {
        if ($taskFile.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Linked source file is not permitted.' }
        $taskEntryName = $taskFile.FullName.Substring($taskStage.Length + 1).Replace('\', '/')
        [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($taskZip, $taskFile.FullName, $taskEntryName, [IO.Compression.CompressionLevel]::Optimal) | Out-Null
    }
} finally { $taskZip.Dispose() }
Assert-CleanSource
if (((Invoke-SourceGit @('rev-parse', 'HEAD')) -join '') -cne $taskCommit) { throw 'Source HEAD changed while archiving.' }
$taskManifest = $taskProvenance
$taskManifest.archive = $taskArchiveName
$taskManifest.archiveSha256 = Get-SourceHash $taskArchivePath
$taskManifest.archiveBytes = (Get-Item -LiteralPath $taskArchivePath).Length
[IO.File]::WriteAllText((Join-Path $taskOutput 'source-manifest.json'), ($taskManifest | ConvertTo-Json -Depth 6) + "`n", $taskUtf8)
[IO.File]::WriteAllText((Join-Path $taskOutput 'SHA256SUMS.txt'), "$($taskManifest.archiveSha256)  $taskArchiveName`n", $taskUtf8)
Remove-Item -LiteralPath $taskGitArchive
Write-Host "Prepared source archive: $taskArchivePath"
Write-Host "Source manifest: $(Join-Path $taskOutput 'source-manifest.json')"
