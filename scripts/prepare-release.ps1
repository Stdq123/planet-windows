# Prepare local files only. This script never installs, tags, pushes or publishes.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [switch]$Release,
    [string]$Tag = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
foreach ($taskModule in @('Utility', 'Management', 'Security')) {
    Import-Module (Join-Path $PSHOME "Modules/Microsoft.PowerShell.$taskModule/Microsoft.PowerShell.$taskModule.psd1") -Force -ErrorAction Stop
}
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskBundle = (Resolve-Path -LiteralPath $BundleDir).Path
$taskUtf8 = [Text.UTF8Encoding]::new($false)
$taskMetadataArgs = @((Join-Path $PSScriptRoot 'release-metadata.ts'))
if (-not $Release -and $Tag) { throw 'Use -Tag only together with -Release.' }
if ($Release) { $taskMetadataArgs += @('--release', '--tag', $Tag) }
$taskMetadataJson = & bun @taskMetadataArgs
if ($LASTEXITCODE -ne 0) { throw 'Release metadata validation failed.' }
$taskMetadata = ($taskMetadataJson -join "`n") | ConvertFrom-Json

$taskCommit = & git -C $taskRoot rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Cannot identify the source checkout.' }
$taskStatus = @(& git -C $taskRoot status --porcelain=v1 --untracked-files=all)
if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect the source checkout.' }
if ($Release) {
    if ($taskStatus.Count -gt 0) { throw 'Release preparation requires a clean checkout.' }
    $taskTagType = & git -C $taskRoot cat-file -t "refs/tags/$Tag"
    if ($LASTEXITCODE -ne 0 -or $taskTagType -ne 'tag') { throw 'Release requires an existing annotated tag.' }
    $taskTagCommit = & git -C $taskRoot rev-parse "refs/tags/$Tag^{commit}"
    if ($LASTEXITCODE -ne 0 -or $taskTagCommit -ne $taskCommit) { throw 'Release tag must point to the checked-out commit.' }
}

# Reuse the installer/payload audit instead of duplicating its file allowlist.
$taskAuditJson = & (Join-Path $taskRoot 'windows/scripts/audit-release.ps1') -BundleDir $taskBundle
$taskAudit = ($taskAuditJson -join "`n") | ConvertFrom-Json
$taskUnsigned = @($taskAudit.signatures | Where-Object {
    $_.file -in @('Planet Windows.exe', $taskAudit.installer) -and $_.status -ne 'Valid'
})
if ($Release -and ($taskUnsigned.Count -gt 0 -or $taskAudit.kuboModulesRequiringManualLicenseReview -gt 0)) {
    throw 'Release blocked: first-party Authenticode signatures or Kubo license reviews are unresolved. Draft preparation remains available.'
}

$taskOutput = Join-Path $taskRoot ('windows/.mygo/release-drafts/' + $taskMetadata.version + '-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($taskOutput) | Out-Null
$taskSourceInstaller = Join-Path $taskBundle $taskAudit.installer
$taskOutputInstaller = Join-Path $taskOutput $taskAudit.installer
Copy-Item -LiteralPath $taskSourceInstaller -Destination $taskOutputInstaller
$taskCopiedHash = (Get-FileHash -LiteralPath $taskOutputInstaller -Algorithm SHA256).Hash.ToLowerInvariant()
if ($taskCopiedHash -ne $taskAudit.installerSha256) { throw 'Installer changed after auditing; discard this incomplete output.' }

$taskManifest = [ordered]@{
    schemaVersion = 1
    mode = $taskMetadata.mode
    version = $taskMetadata.version
    prerelease = $taskMetadata.prerelease
    tag = $taskMetadata.tag
    sourceCommitAtPreparation = $taskCommit
    sourceCheckoutDirty = ($taskStatus.Count -gt 0)
    preparedAtUtc = [DateTime]::UtcNow.ToString('o')
    installer = $taskAudit.installer
    installerSha256 = $taskCopiedHash
    installerBytes = (Get-Item -LiteralPath $taskOutputInstaller).Length
    signatures = $taskAudit.signatures
    kuboModulesRequiringManualLicenseReview = $taskAudit.kuboModulesRequiringManualLicenseReview
    note = 'Checkout identity is not a build attestation. Native Windows acceptance and complete license review remain required.'
}
[IO.File]::WriteAllText((Join-Path $taskOutput 'release-manifest.json'), ($taskManifest | ConvertTo-Json -Depth 6) + "`n", $taskUtf8)
[IO.File]::WriteAllText((Join-Path $taskOutput 'audit.json'), ($taskAuditJson -join "`n") + "`n", $taskUtf8)
[IO.File]::WriteAllText((Join-Path $taskOutput 'SHA256SUMS.txt'), "$taskCopiedHash  $($taskAudit.installer)`n", $taskUtf8)
[IO.File]::WriteAllText((Join-Path $taskOutput 'release-notes.md'), $taskMetadata.notes, $taskUtf8)
if (-not $Release) { Write-Warning 'DRAFT ONLY: not approved for public distribution. Review audit.json and the release checklist.' }
Write-Host "Prepared local release materials: $taskOutput"
