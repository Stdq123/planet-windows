# Prepare local files only. This script never installs, tags, pushes or publishes.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BundleDir,
    [switch]$Release,
    [string]$Tag = '',
    [switch]$AllowUnsignedPrerelease
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
if ($AllowUnsignedPrerelease -and (-not $Release -or -not $taskMetadata.prerelease)) {
    throw 'AllowUnsignedPrerelease requires -Release and a prerelease version.'
}

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
if ($null -eq $taskAudit.PSObject.Properties['signatures']) {
    throw 'Release blocked: audit is missing signature records.'
}
$taskSignatureRecords = @($taskAudit.signatures)
foreach ($taskEntry in $taskSignatureRecords) {
    if ($null -eq $taskEntry -or $null -eq $taskEntry.PSObject.Properties['file'] -or $null -eq $taskEntry.PSObject.Properties['status']) {
        throw 'Release blocked: malformed signature audit record.'
    }
}
$taskFirstPartySignatures = @()
$taskCompleteSignatures = $true
foreach ($taskFile in @('Planet Windows.exe', $taskAudit.installer)) {
    $taskEntries = @($taskSignatureRecords | Where-Object { $_.file -ieq $taskFile })
    if ($taskEntries.Count -ne 1) { $taskCompleteSignatures = $false }
    $taskFirstPartySignatures += $taskEntries
}
$taskUnsigned = @($taskFirstPartySignatures | Where-Object { $_.status -ceq 'NotSigned' })
$taskAllowedStatuses = if ($AllowUnsignedPrerelease) { @('Valid', 'NotSigned') } else { @('Valid') }
$taskRejectedSignatures = @($taskFirstPartySignatures | Where-Object { $_.status -cnotin $taskAllowedStatuses })
if ($Release) {
    if (-not $taskCompleteSignatures -or $taskRejectedSignatures.Count -gt 0) {
        throw 'Release blocked: require exactly one acceptable Authenticode audit record for the app and installer. Only an explicit unsigned prerelease may use NotSigned.'
    }
    if ($taskAudit.kuboModulesRequiringManualLicenseReview -gt 0) {
        throw 'Release blocked: Kubo license reviews are unresolved. Draft preparation remains available.'
    }
}
$taskUnsignedPrerelease = [bool]($AllowUnsignedPrerelease -and $taskUnsigned.Count -gt 0)
$taskSigningPolicy = if ($AllowUnsignedPrerelease) { 'allow-unsigned-prerelease' } elseif ($Release) { 'require-valid-authenticode' } else { 'draft-audit-only' }
$taskOtherSignatures = @($taskFirstPartySignatures | Where-Object { $_.status -cnotin @('Valid', 'NotSigned') })
$taskSigningStatus = if (-not $taskCompleteSignatures -or $taskOtherSignatures.Count -gt 0) { 'unverified' } elseif ($taskUnsigned.Count -gt 0) { 'unsigned' } else { 'valid' }

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
    unsignedPrerelease = $taskUnsignedPrerelease
    signingPolicy = $taskSigningPolicy
    firstPartySigningStatus = $taskSigningStatus
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
$taskReleaseNotes = $taskMetadata.notes
if ($taskUnsignedPrerelease) {
    $taskSigningNotice = "`n## UNSIGNED PRERELEASE`n`nThis prerelease includes an application or installer without a Windows Authenticode signature. SHA-256 checks verify file integrity; they do not authenticate publisher identity. Windows may show security prompts or block execution. Review the source and release audit before using this experimental build.`n"
    $taskReleaseNotes = $taskReleaseNotes.Insert($taskReleaseNotes.IndexOf("`n") + 1, $taskSigningNotice)
}
[IO.File]::WriteAllText((Join-Path $taskOutput 'release-notes.md'), $taskReleaseNotes, $taskUtf8)
if (-not $Release) { Write-Warning 'DRAFT ONLY: not approved for public distribution. Review audit.json and the release checklist.' }
if ($taskUnsignedPrerelease) { Write-Warning 'UNSIGNED PRERELEASE: review the signing policy and notice in the release materials.' }
Write-Host "Prepared local release materials: $taskOutput"
