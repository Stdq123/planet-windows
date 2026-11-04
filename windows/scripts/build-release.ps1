param()
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Utility/Microsoft.PowerShell.Utility.psd1') -Force -ErrorAction Stop
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Management/Microsoft.PowerShell.Management.psd1') -Force -ErrorAction Stop
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Security/Microsoft.PowerShell.Security.psd1') -Force -ErrorAction Stop
$taskProjectRoot = [System.IO.Directory]::GetParent($PSScriptRoot).FullName
$taskReleaseVersion=([IO.File]::ReadAllText((Join-Path $taskProjectRoot 'release.json')) | ConvertFrom-Json).version
if ($taskReleaseVersion -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid release version.' }
$taskBuildId = [Guid]::NewGuid().ToString('N')
$taskStageRoot = Join-Path $taskProjectRoot ('.mygo/build-staging/' + $taskBuildId)
$taskReleaseRoot = Join-Path $taskProjectRoot ('.mygo/releases/' + $taskBuildId + '/windows-amd64')
[System.IO.Directory]::CreateDirectory($taskStageRoot) | Out-Null
[System.IO.Directory]::CreateDirectory($taskReleaseRoot) | Out-Null
Push-Location $taskProjectRoot
try {
    & (Join-Path $PSScriptRoot 'prepare-kubo.ps1')
    & bunx mygo build -platform windows/amd64 -o $taskStageRoot
    if ($LASTEXITCODE -ne 0) { throw 'MyGo build failed; no installer was delivered.' }
    $taskPayload = Join-Path $taskStageRoot 'windows-amd64'
    # Read the actual embedded version in an isolated profile before packaging.
    $taskVersionOutput=Join-Path $taskStageRoot 'app-version.txt'
    $taskPreviousAppData=$env:APPDATA
    $taskPreviousLocal=$env:LOCALAPPDATA
    try {
        $env:APPDATA=Join-Path $taskStageRoot 'version-profile'
        $env:LOCALAPPDATA=$env:APPDATA
        # Windows PowerShell's Start-Process may expose a null ExitCode for a
        # fast GUI process. Own the native Process handle through completion.
        $taskVersionProcess=New-Object System.Diagnostics.Process
        $taskVersionProcess.StartInfo.FileName=Join-Path $taskPayload 'Planet Windows.exe'
        $taskVersionProcess.StartInfo.Arguments='--planet-version'
        $taskVersionProcess.StartInfo.WorkingDirectory=$taskPayload
        $taskVersionProcess.StartInfo.UseShellExecute=$false
        $taskVersionProcess.StartInfo.CreateNoWindow=$true
        $taskVersionProcess.StartInfo.RedirectStandardOutput=$true
        if (-not $taskVersionProcess.Start()) { throw 'Could not start the built app version check.' }
        if (-not $taskVersionProcess.WaitForExit(10000)) { $taskVersionProcess.Kill(); throw 'Built app did not respond to the read-only version check.' }
        $taskObservedExit=$taskVersionProcess.ExitCode
        $taskObservedVersion=$taskVersionProcess.StandardOutput.ReadToEnd().Trim()
        [IO.File]::WriteAllText($taskVersionOutput,$taskObservedVersion,[Text.UTF8Encoding]::new($false))
        if ($taskObservedExit -ne 0 -or $taskObservedVersion -ne $taskReleaseVersion) { throw "Built app version check failed: exit=[$taskObservedExit], observed=[$taskObservedVersion], expected=[$taskReleaseVersion]." }
    } finally { if($taskVersionProcess){$taskVersionProcess.Dispose()}; $env:APPDATA=$taskPreviousAppData; $env:LOCALAPPDATA=$taskPreviousLocal }
    $taskFiles = @('Planet Windows.exe','ipfs.exe','icon.png','kubo-LICENSE.txt','kubo-LICENSE-MIT.txt','kubo-LICENSE-APACHE.txt','LICENSE-PLANET.txt','THIRD-PARTY-NOTICES.txt','KUBO-BUILDINFO.txt','KUBO-DEPENDENCY-LICENSES.txt')
    foreach ($taskName in $taskFiles) {
        $taskSource = Join-Path $taskPayload $taskName
        if (-not (Test-Path -LiteralPath $taskSource -PathType Leaf)) { throw "Build payload missing: $taskName" }
        Copy-Item -LiteralPath $taskSource -Destination (Join-Path $taskReleaseRoot $taskName)
    }
    $taskInstaller = Join-Path $taskReleaseRoot ('planet-windows-safe-setup-' + $taskReleaseVersion + '-amd64.exe')
    & (Join-Path $PSScriptRoot 'build-safe-installer.ps1') -PayloadDir $taskReleaseRoot -OutputFile $taskInstaller
    & (Join-Path $PSScriptRoot 'audit-release.ps1') -BundleDir $taskReleaseRoot
    & (Join-Path $PSScriptRoot 'publish-build.ps1') -BundleDir $taskReleaseRoot -BuildDir (Join-Path $taskProjectRoot 'build') -InstallerName ([IO.Path]::GetFileName($taskInstaller))
    Write-Host "Audited release directory: $taskReleaseRoot"
} finally { Pop-Location }
