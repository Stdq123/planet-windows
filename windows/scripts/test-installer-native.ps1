# Executes a real installer. Run only in a disposable Windows runner or an
# explicitly selected clean Windows VM, never in a daily-use Windows profile.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [Parameter(Mandatory = $true)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$ExpectedSHA256,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion,
    [Parameter(Mandatory = $true)][string]$EvidenceDir,
    [switch]$IsolatedWindowsVM
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
# The extra marker prevents normal CI quality checks from executing installers.
# This is an operator guard, not a security boundary against forged environment.
$taskHostedRunner = $env:GITHUB_ACTIONS -ceq 'true' -and $env:RUNNER_OS -ceq 'Windows' -and
    $env:RUNNER_ENVIRONMENT -ceq 'github-hosted' -and $env:PLANET_NATIVE_INSTALLER_SMOKE -ceq '1'
if (-not $taskHostedRunner -and -not $IsolatedWindowsVM) {
    throw 'Native installer smoke requires an explicitly marked GitHub-hosted Windows runner or -IsolatedWindowsVM. Refusing normal host execution.'
}
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Native Windows is required.' }
foreach ($taskModule in @('Utility', 'Management', 'Security')) {
    Import-Module (Join-Path $PSHOME "Modules/Microsoft.PowerShell.$taskModule/Microsoft.PowerShell.$taskModule.psd1") -Force -ErrorAction Stop
}
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$taskUtf8 = [Text.UTF8Encoding]::new($false)

function Assert-Native([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
function Assert-NormalPath([string]$Path) {
    for ($taskAncestor = [IO.Path]::GetFullPath($Path); $taskAncestor; $taskAncestor = [IO.Path]::GetDirectoryName($taskAncestor)) {
        if (Test-Path -LiteralPath $taskAncestor) {
            Assert-Native (-not ((Get-Item -LiteralPath $taskAncestor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) "Reparse point in test path: $taskAncestor"
        }
    }
}
function Get-NativeHash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

$taskInstaller = (Resolve-Path -LiteralPath $Installer).Path
Assert-NormalPath $taskInstaller
Assert-Native (Test-Path -LiteralPath $taskInstaller -PathType Leaf) 'Installer must be a normal file.'
Assert-Native ((Get-NativeHash $taskInstaller) -ceq $ExpectedSHA256.ToLowerInvariant()) 'Installer SHA-256 differs from expected bytes.'
$taskVersionJson = & bun (Join-Path $PSScriptRoot '../../scripts/release-version.ts') $ExpectedVersion
if ($LASTEXITCODE -ne 0) { throw 'Expected version is invalid.' }
Assert-Native (($taskVersionJson | ConvertFrom-Json).version -ceq $ExpectedVersion) 'Expected version is not canonical.'
$taskBundle = [IO.Path]::GetDirectoryName($taskInstaller)
$taskPayloadNames = @('Planet Windows.exe', 'ipfs.exe', 'icon.png', 'kubo-LICENSE.txt', 'kubo-LICENSE-MIT.txt', 'kubo-LICENSE-APACHE.txt', 'LICENSE-PLANET.txt', 'THIRD-PARTY-NOTICES.txt', 'KUBO-BUILDINFO.txt', 'KUBO-DEPENDENCY-LICENSES.txt')
$taskExpectedHashes = [ordered]@{}
foreach ($taskName in $taskPayloadNames) {
    $taskFile = Join-Path $taskBundle $taskName
    Assert-NormalPath $taskFile
    $taskExpectedHashes[$taskName] = Get-NativeHash $taskFile
}

$taskStateKey = 'HKCU:\Software\PlanetWindows\InstallerState'
$taskUninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows'
$taskTransactionKey = 'HKCU:\Software\PlanetWindows\UpdateTransaction'
$taskUserData = Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'Planet Windows'
$taskLocalData = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Planet Windows'
$taskDefaultInstall = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs/Planet Windows'
$taskShortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'Planet Windows.lnk'
foreach ($taskPath in @($taskStateKey, $taskUninstallKey, $taskTransactionKey, $taskUserData, $taskLocalData, $taskDefaultInstall, $taskShortcut)) {
    Assert-Native (-not (Test-Path -LiteralPath $taskPath)) "Test requires a clean disposable profile; existing product path: $taskPath"
}
Assert-Native (@(Get-Process -Name 'Planet Windows' -ErrorAction SilentlyContinue).Count -eq 0) 'A Planet process already exists.'
foreach ($taskPath in @($taskUserData, $taskLocalData, $taskShortcut, $EvidenceDir)) { Assert-NormalPath $taskPath }

$taskTempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$taskRoot = Join-Path $taskTempBase ('planet-native-installer-' + [guid]::NewGuid().ToString('N'))
Assert-NormalPath $taskRoot
Assert-Native (-not (Test-Path -LiteralPath $taskRoot)) 'Test root already exists.'
$taskInstall = Join-Path $taskRoot 'installed'
$taskForeign = Join-Path $taskRoot 'nonempty'
$taskUserSentinel = Join-Path $taskUserData 'native-smoke-synthetic.txt'
$taskUnknown = Join-Path $taskInstall 'unknown-user-file.txt'
$taskEvents = [Collections.Generic.List[object]]::new()
$taskEvidence = [ordered]@{
    schemaVersion = 1
    status = 'running'
    scope = 'Native silent installer smoke; not Windows 10/11 GUI, WebView2, IME, real upgrade compatibility or power-loss acceptance.'
    installer = [IO.Path]::GetFileName($taskInstaller)
    installerSha256 = $ExpectedSHA256.ToLowerInvariant()
    version = $ExpectedVersion
    sourceCommit = $env:GITHUB_SHA
    runnerImage = $env:ImageOS
    runnerImageVersion = $env:ImageVersion
    windows = [Environment]::OSVersion.VersionString
    startedAtUtc = [DateTime]::UtcNow.ToString('o')
    testRoot = $taskRoot
    events = $taskEvents
}
$taskOutput = [IO.Path]::GetFullPath($EvidenceDir)
[IO.Directory]::CreateDirectory($taskOutput) | Out-Null
$taskEvidenceFile = Join-Path $taskOutput ('native-installer-' + [guid]::NewGuid().ToString('N') + '.json')
[IO.Directory]::CreateDirectory($taskRoot) | Out-Null

function Add-NativeResult([string]$Name) {
    $taskEvents.Add([ordered]@{ name = $Name; status = 'passed'; atUtc = [DateTime]::UtcNow.ToString('o') })
    Write-Host "PASS: $Name"
}
function Invoke-NativeSetup([string]$Destination, [bool]$Success) {
    # NSIS /D must be last and consumes the remainder of the command line.
    Assert-Native ($Destination -notmatch '["\r\n]') 'Unsafe installer destination argument.'
    # Own the native handle: Windows PowerShell Start-Process can lose the exit
    # code of a fast failing GUI installer. Its guard children are synchronous.
    $taskProcess = [Diagnostics.Process]::new()
    try {
        $taskProcess.StartInfo.FileName = $taskInstaller
        $taskProcess.StartInfo.Arguments = "/S /D=$Destination"
        $taskProcess.StartInfo.WorkingDirectory = $taskRoot
        $taskProcess.StartInfo.UseShellExecute = $false
        $taskProcess.StartInfo.CreateNoWindow = $true
        Assert-Native ($taskProcess.Start()) 'Installer did not start.'
        if (-not $taskProcess.WaitForExit(120000)) { $taskProcess.Kill(); throw 'Installer exceeded the two-minute smoke-test timeout.' }
        if ($Success) { Assert-Native ($taskProcess.ExitCode -eq 0) "Installation failed: $($taskProcess.ExitCode)" }
        else { Assert-Native ($taskProcess.ExitCode -ne 0) 'Installer accepted a forbidden operation.' }
    } finally { $taskProcess.Dispose() }
}
function Invoke-NativeUninstall {
    # On Windows -Wait waits for descendants, including NSIS's temporary copy.
    # Do not trust the original launcher's exit code as the uninstall verdict.
    # Bound that descendant wait in a background job so a hung uninstaller
    # cannot prevent the outer finally block from saving failure evidence.
    $taskUninstallJob = Start-Job -ArgumentList (Join-Path $taskInstall 'Uninstall.exe'), $taskRoot -ScriptBlock {
        param($taskUninstaller, $taskWorkingDirectory)
        $ErrorActionPreference = 'Stop'
        $taskUninstallProcess = $null
        try {
            $taskUninstallProcess = Start-Process -FilePath $taskUninstaller -ArgumentList '/S' -WorkingDirectory $taskWorkingDirectory -WindowStyle Hidden -PassThru -Wait
        } finally {
            if ($taskUninstallProcess) { $taskUninstallProcess.Dispose() }
        }
    }
    try {
        if (-not (Wait-Job -Job $taskUninstallJob -Timeout 120)) {
            Stop-Job -Job $taskUninstallJob
            throw 'Uninstaller or its descendants exceeded the two-minute smoke-test timeout.'
        }
        Receive-Job -Job $taskUninstallJob -ErrorAction Stop | Out-Null
    } finally {
        if ($taskUninstallJob.State -eq 'Running') { Stop-Job -Job $taskUninstallJob }
        Remove-Job -Job $taskUninstallJob -Force
    }
}
function Assert-Installed {
    foreach ($taskName in $taskPayloadNames) {
        Assert-Native ((Get-NativeHash (Join-Path $taskInstall $taskName)) -ceq $taskExpectedHashes[$taskName]) "Installed payload mismatch: $taskName"
    }
    Assert-Native (Test-Path -LiteralPath (Join-Path $taskInstall 'Uninstall.exe') -PathType Leaf) 'Uninstaller missing.'
    $taskRegistration = Get-ItemProperty -LiteralPath $taskUninstallKey
    Assert-Native ($taskRegistration.DisplayVersion -ceq $ExpectedVersion) 'Registered version differs.'
    Assert-Native ($taskRegistration.InstallLocation -ieq $taskInstall) 'Registration points outside the test installation.'
    Assert-Native ((Get-ItemProperty -LiteralPath $taskStateKey).TransactionProtocol -ceq '2') 'Installer transaction protocol missing.'
    Assert-Native (-not (Test-Path -LiteralPath $taskTransactionKey)) 'Unexpected pending transaction.'
}
function Get-InstalledSnapshot {
    $taskSnapshot = [ordered]@{}
    foreach ($taskName in ($taskPayloadNames + @('Uninstall.exe', 'unknown-user-file.txt'))) {
        $taskSnapshot[$taskName] = Get-NativeHash (Join-Path $taskInstall $taskName)
    }
    $taskSnapshot.shortcut = Get-NativeHash $taskShortcut
    $taskSnapshot.userData = Get-NativeHash $taskUserSentinel
    return ($taskSnapshot | ConvertTo-Json -Compress)
}

try {
    $taskOS = Get-CimInstance Win32_OperatingSystem
    $taskEvidence.windows = "$($taskOS.Caption) $($taskOS.Version) build $($taskOS.BuildNumber)"
    $taskEvidence.signatures = @('Planet Windows.exe', [IO.Path]::GetFileName($taskInstaller)) | ForEach-Object {
        $taskSignature = Get-AuthenticodeSignature -LiteralPath (Join-Path $taskBundle $_)
        [ordered]@{ file = $_; status = [string]$taskSignature.Status }
    }
    [IO.Directory]::CreateDirectory($taskUserData) | Out-Null
    [IO.File]::WriteAllText($taskUserSentinel, 'Synthetic user data to preserve; no real site or key.', $taskUtf8)
    $taskUserHash = Get-NativeHash $taskUserSentinel
    [IO.Directory]::CreateDirectory($taskForeign) | Out-Null
    $taskForeignFile = Join-Path $taskForeign 'Planet Windows.exe'
    [IO.File]::WriteAllText($taskForeignFile, 'Foreign test file; not executable.', $taskUtf8)
    $taskForeignHash = Get-NativeHash $taskForeignFile
    Invoke-NativeSetup $taskForeign $false
    Assert-Native ((Get-NativeHash $taskForeignFile) -ceq $taskForeignHash) 'Rejected installation changed foreign content.'
    Assert-Native (-not (Test-Path -LiteralPath $taskStateKey)) 'Rejected installation created ownership.'
    Add-NativeResult 'nonempty-directory-refused'

    Invoke-NativeSetup $taskInstall $true
    Assert-Installed
    Assert-Native (Test-Path -LiteralPath $taskShortcut -PathType Leaf) 'Start Menu shortcut missing.'
    $taskShell = New-Object -ComObject WScript.Shell
    try { Assert-Native ($taskShell.CreateShortcut($taskShortcut).TargetPath -ieq (Join-Path $taskInstall 'Planet Windows.exe')) 'Shortcut target differs.' }
    finally { [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($taskShell) }
    Add-NativeResult 'fresh-install-payload-registration-shortcut'

    $taskVersionProcess = [Diagnostics.Process]::new()
    try {
        $taskVersionProcess.StartInfo.FileName = Join-Path $taskInstall 'Planet Windows.exe'
        $taskVersionProcess.StartInfo.Arguments = '--planet-version'
        $taskVersionProcess.StartInfo.UseShellExecute = $false
        $taskVersionProcess.StartInfo.CreateNoWindow = $true
        $taskVersionProcess.StartInfo.RedirectStandardOutput = $true
        Assert-Native ($taskVersionProcess.Start()) 'Installed program did not start.'
        if (-not $taskVersionProcess.WaitForExit(10000)) { $taskVersionProcess.Kill(); throw 'Installed version probe timed out.' }
        Assert-Native ($taskVersionProcess.ExitCode -eq 0 -and $taskVersionProcess.StandardOutput.ReadToEnd().Trim() -ceq $ExpectedVersion) 'Installed program returned a different version.'
    } finally { $taskVersionProcess.Dispose() }
    Add-NativeResult 'installed-program-version'

    [IO.File]::WriteAllText($taskUnknown, 'Unknown user file must survive reinstall and uninstall.', $taskUtf8)
    $taskSnapshot = Get-InstalledSnapshot
    Invoke-NativeSetup $taskInstall $true
    Assert-Installed
    Assert-Native ((Get-InstalledSnapshot) -ceq $taskSnapshot) 'Same-package reinstall changed files or data.'
    Add-NativeResult 'same-package-reinstall-no-op'

    $taskLockedFile = Join-Path $taskInstall 'ipfs.exe'
    $taskLock = [IO.File]::Open($taskLockedFile, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try {
        Invoke-NativeSetup $taskInstall $false
        Invoke-NativeUninstall
        Assert-Native (Test-Path -LiteralPath (Join-Path $taskInstall 'Uninstall.exe')) 'Locked uninstall removed its entry point.'
        Assert-Native (Test-Path -LiteralPath $taskUninstallKey) 'Locked uninstall removed registration.'
    } finally { $taskLock.Dispose() }
    Assert-Installed
    Assert-Native ((Get-InstalledSnapshot) -ceq $taskSnapshot) 'Refused locked operation changed files or data.'
    Add-NativeResult 'locked-file-reinstall-and-uninstall-refused'

    $taskIcon = Join-Path $taskInstall 'icon.png'
    $taskOriginalIcon = [IO.File]::ReadAllBytes($taskIcon)
    try {
        [IO.File]::WriteAllText($taskIcon, 'Synthetic modified owned file.', $taskUtf8)
        $taskTamperedSnapshot = Get-InstalledSnapshot
        Invoke-NativeSetup $taskInstall $false
        Invoke-NativeUninstall
        Assert-Native ((Get-InstalledSnapshot) -ceq $taskTamperedSnapshot) 'Refused tampered operation changed content.'
        Assert-Native (Test-Path -LiteralPath $taskUninstallKey) 'Tampered uninstall removed registration.'
    } finally { [IO.File]::WriteAllBytes($taskIcon, $taskOriginalIcon) }
    Assert-Installed
    Add-NativeResult 'tampered-file-reinstall-and-uninstall-refused'

    $taskUnknownHash = Get-NativeHash $taskUnknown
    Invoke-NativeUninstall
    foreach ($taskName in ($taskPayloadNames + @('Uninstall.exe'))) {
        Assert-Native (-not (Test-Path -LiteralPath (Join-Path $taskInstall $taskName))) "Uninstall retained owned file: $taskName"
    }
    foreach ($taskPath in @($taskStateKey, $taskUninstallKey, $taskTransactionKey, $taskShortcut)) {
        Assert-Native (-not (Test-Path -LiteralPath $taskPath)) "Uninstall retained ownership or shortcut: $taskPath"
    }
    Assert-Native ((Get-NativeHash $taskUnknown) -ceq $taskUnknownHash) 'Uninstall removed or changed unknown data.'
    Assert-Native ((Get-NativeHash $taskUserSentinel) -ceq $taskUserHash) 'Uninstall changed synthetic user data.'
    Add-NativeResult 'uninstall-descendants-complete-owned-files-removed-user-data-preserved'
    $taskEvidence.status = 'passed'
} catch {
    $taskEvidence.status = 'failed'
    $taskEvidence.error = $_.Exception.Message
    throw
} finally {
    $taskEvidence.finishedAtUtc = [DateTime]::UtcNow.ToString('o')
    [IO.File]::WriteAllText($taskEvidenceFile, ($taskEvidence | ConvertTo-Json -Depth 8) + "`n", $taskUtf8)
    Write-Host "Native installer evidence: $taskEvidenceFile"
    Write-Host "Test files retained for inspection: $taskRoot"
}
