[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [string]$InstallerName = '',
    [string]$SevenZip = 'C:\Program Files\7-Zip\7z.exe'
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Utility/Microsoft.PowerShell.Utility.psd1') -Force -ErrorAction Stop
Import-Module (Join-Path $PSHOME 'Modules/Microsoft.PowerShell.Security/Microsoft.PowerShell.Security.psd1') -Force -ErrorAction Stop
if (-not $InstallerName) { $InstallerName = 'planet-windows-safe-setup-' + (([IO.File]::ReadAllText((Join-Path ([IO.Directory]::GetParent($PSScriptRoot).FullName) 'release.json')) | ConvertFrom-Json).version) + '-amd64.exe' }
$bundle = (Resolve-Path -LiteralPath $BundleDir).Path
if (-not (Test-Path -LiteralPath $SevenZip -PathType Leaf)) { throw "7-Zip not found: $SevenZip" }
if ([System.IO.Path]::GetFileName($InstallerName) -ne $InstallerName) { throw 'InstallerName must be a file name, not a path.' }
$installerName = $InstallerName
$installer = Join-Path $bundle $installerName
$files = @('Planet Windows.exe', 'icon.png', 'ipfs.exe', 'kubo-LICENSE.txt', 'kubo-LICENSE-MIT.txt', 'kubo-LICENSE-APACHE.txt', 'LICENSE-PLANET.txt', 'THIRD-PARTY-NOTICES.txt', 'KUBO-BUILDINFO.txt', 'KUBO-DEPENDENCY-LICENSES.txt')
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw "Installer not found: $installer" }

$header = & $SevenZip l -slt $installer
if ($LASTEXITCODE -ne 0 -or -not ($header -contains 'Type = Nsis')) { throw 'Installer is not a readable NSIS archive.' }
$listing = & $SevenZip l -slt -ba $installer
if ($LASTEXITCODE -ne 0) { throw 'Cannot read the installer with 7-Zip.' }
$entries = @($listing | Where-Object { $_ -like 'Path = *' } | ForEach-Object { $_.Substring(7) })
if (@($entries | Where-Object { -not $_.StartsWith('$PLUGINSDIR\') -and $_ -notin $files }).Count) {
    throw 'Installer has unexpected payload entries; audit the changed payload and uninstall manifest.'
}

function Get-ArchiveEntryHash([string]$Name) {
    $p = New-Object System.Diagnostics.Process
    $p.StartInfo.FileName = $SevenZip
    # Windows paths cannot contain a double quote. Both arguments are quoted;
    # 7-Zip's -- ends its options so a payload name cannot become a flag.
    $p.StartInfo.Arguments = 'x -so -bd -bsp0 -bse0 -- "' + $installer + '" "' + $Name + '"'
    $p.StartInfo.UseShellExecute = $false
    $p.StartInfo.CreateNoWindow = $true
    $p.StartInfo.RedirectStandardOutput = $true
    if (-not $p.Start()) { throw "Failed to start 7-Zip for $Name" }
    try {
        $hash = [Security.Cryptography.SHA256]::Create()
        try { $digest = $hash.ComputeHash($p.StandardOutput.BaseStream) } finally { $hash.Dispose() }
        $p.WaitForExit()
        if ($p.ExitCode -ne 0) { throw "Could not read installer entry $Name (7-Zip exit $($p.ExitCode))." }
        return [BitConverter]::ToString($digest).Replace('-', '').ToLowerInvariant()
    } finally { $p.Dispose() }
}

$results = @()
foreach ($name in $files) {
    if (@($entries | Where-Object { $_ -eq $name }).Count -ne 1) { throw "Installer must have exactly one $name entry." }
    $file = Join-Path $bundle $name
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Bundle file is missing: $file" }
    $diskHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
    $archiveHash = Get-ArchiveEntryHash $name
    if ($archiveHash -ne $diskHash) { throw "Installer and standalone payload differ: $name" }
    $results += [pscustomobject]@{ file = $name; sha256 = $diskHash }
}
$signatures = @('Planet Windows.exe', $installerName, 'ipfs.exe') | ForEach-Object {
    $result = Get-AuthenticodeSignature -LiteralPath (Join-Path $bundle $_)
    [pscustomobject]@{ file = $_; status = [string]$result.Status }
}
$manualReviews = @([System.IO.File]::ReadLines((Join-Path $bundle 'KUBO-DEPENDENCY-LICENSES.txt')) | Where-Object { $_ -like 'REQUIRES MANUAL REVIEW:*' }).Count
[pscustomobject]@{
    installer = $installerName
    installerSha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $installer).Hash.ToLowerInvariant()
    payload = $results
    signatures = $signatures
    kuboModulesRequiringManualLicenseReview = $manualReviews
    note = 'Payload equality does not prove install/uninstall safety, Windows execution, or license compliance.'
} | ConvertTo-Json -Depth 6
