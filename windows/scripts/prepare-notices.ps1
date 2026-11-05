param([string]$Destination)
$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Directory]::GetParent($PSScriptRoot).FullName
if (-not $Destination) { $Destination = Join-Path $projectRoot 'resources/windows-amd64' }
$Destination = [System.IO.Path]::GetFullPath($Destination)
$utf8 = New-Object System.Text.UTF8Encoding($false)

# Enumerate the packages actually linked for the release target, not all
# modules in go.mod (which can include tests and build-only dependencies).
$previous = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Push-Location $projectRoot
    try {
        $lines = & go list -deps -tags=mygo_noinspector -f '{{with .Module}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}' .
        if ($LASTEXITCODE -ne 0) { throw 'go list failed; cannot enumerate release dependencies.' }
        $goRoot = & go env GOROOT
        if ($LASTEXITCODE -ne 0 -or -not $goRoot) { throw 'go env GOROOT failed.' }
        $mygoVersion = & go list -m -f '{{.Version}}' github.com/egoist/mygo
        if ($LASTEXITCODE -ne 0 -or $mygoVersion -ne 'v0.7.4') { throw 'The MyGo version changed; re-audit the installer hook before building.' }
    } finally { Pop-Location }
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}

$package = Get-Content -Raw -Encoding UTF8 (Join-Path $projectRoot 'package.json') | ConvertFrom-Json
$runtimeNames = @($package.dependencies.PSObject.Properties.Name | Sort-Object)
if ($runtimeNames.Count -ne 1 -or $runtimeNames[0] -ne 'mygo-runtime') {
    throw 'Runtime frontend dependencies changed; update the notice collector before distributing.'
}
$runtimeDir = Join-Path $projectRoot 'node_modules/mygo-runtime'
$runtimePackage = Get-Content -Raw -Encoding UTF8 (Join-Path $runtimeDir 'package.json') | ConvertFrom-Json
$cliPackage = Get-Content -Raw -Encoding UTF8 (Join-Path $projectRoot 'node_modules/mygo-cli/package.json') | ConvertFrom-Json
if ($cliPackage.version -ne '0.7.4') { throw 'The MyGo CLI version changed; re-audit the installer hook before building.' }
$runtimeLicense = Join-Path $runtimeDir 'LICENSE'
if (-not (Test-Path -LiteralPath $runtimeLicense -PathType Leaf)) { throw 'mygo-runtime LICENSE is missing.' }
$goLicense = Join-Path $goRoot 'LICENSE'
if (-not (Test-Path -LiteralPath $goLicense -PathType Leaf)) { throw 'Go LICENSE is missing.' }
$kuboExecutable = Join-Path $projectRoot 'resources/windows-amd64/ipfs.exe'
if (-not (Test-Path -LiteralPath $kuboExecutable -PathType Leaf)) { throw 'Kubo is missing. Run scripts/prepare-kubo.ps1 before preparing release notices.' }
$kuboInfo = & go version -m $kuboExecutable
if ($LASTEXITCODE -ne 0 -or -not ($kuboInfo -match 'github.com/ipfs/kubo')) { throw 'Cannot read the bundled Kubo Go build metadata.' }
$kuboEvidence = Join-Path $projectRoot 'docs/kubo-dependency-licenses.txt'
if (-not (Test-Path -LiteralPath $kuboEvidence -PathType Leaf)) { throw 'Kubo dependency license evidence is missing; run scripts/release-audit/kubo-notices.go.' }
$evidenceLines = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
$manualCount = 0
foreach ($line in [System.IO.File]::ReadLines($kuboEvidence)) {
    [void]$evidenceLines.Add($line)
    if ($line -like 'REQUIRES MANUAL REVIEW:*') { $manualCount++ }
}
$dependencies = @($kuboInfo | Where-Object { $_ -match '^\s*dep\s+(\S+)\s+(\S+)\s+(\S+)\s*$' })
if ($dependencies.Count -eq 0) { throw 'Kubo binary has no readable dependency metadata.' }
foreach ($line in $dependencies) {
    if ($line -match '^\s*dep\s+(\S+)\s+(\S+)\s+(\S+)\s*$') {
        $identity = "$($Matches[1]) $($Matches[2]) $($Matches[3])"
        if (-not $evidenceLines.Contains($identity)) { throw "Kubo license evidence is stale or missing module: $identity" }
    }
}
if ($manualCount -gt 0) { Write-Warning "Kubo license evidence contains $manualCount modules requiring manual review; distribution is not cleared." }

$notice = New-Object System.Text.StringBuilder
[void]$notice.AppendLine('Third-party license materials for Planet Windows (windows/amd64).')
[void]$notice.AppendLine('Generated from the linked Go modules and frontend runtime; assess additional obligations before distribution.')
[void]$notice.AppendLine('Kubo ships separately; see KUBO-BUILDINFO.txt and KUBO-DEPENDENCY-LICENSES.txt. The latter includes entries requiring manual review.')
function Add-License([string]$Title, [string]$Path) {
    [void]$notice.AppendLine()
    [void]$notice.AppendLine(('=' * 72))
    [void]$notice.AppendLine($Title)
    [void]$notice.AppendLine(('=' * 72))
    [void]$notice.AppendLine([System.IO.File]::ReadAllText($Path))
}
Add-License 'Go standard library: LICENSE' $goLicense
Add-License ("mygo-runtime " + $runtimePackage.version + ': LICENSE') $runtimeLicense
Add-License 'Built-in Planet templates: pinned sources and license status' (Join-Path $projectRoot 'internal/templateinstall/builtins/TEMPLATE-NOTICES.txt')
Add-License '8-bit template: upstream icon attribution' (Join-Path $projectRoot 'internal/templateinstall/builtins/gamedb/ATTRIBUTION.md')
Add-License 'Sepia template: LICENSE' (Join-Path $projectRoot 'internal/templateinstall/builtins/sepia/LICENSE')
$modules = @($lines | Where-Object { $_ -and $_ -notlike 'windows|*' } | Sort-Object -Unique)
if ($modules.Count -eq 0) { throw 'No Go dependency modules were found.' }
foreach ($line in $modules) {
    $parts = $line.Split('|', 3)
    if ($parts.Count -ne 3 -or -not $parts[2]) { throw "Unexpected Go module entry: $line" }
    $licenses = @(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object { $_.Name -match '^(?i:LICENSE|COPYING|NOTICE)(\.|$|-)' } | Sort-Object Name)
    if ($licenses.Count -eq 0) { throw "No root license/notice found for $($parts[0]) $($parts[1])." }
    foreach ($file in $licenses) { Add-License ("$($parts[0]) $($parts[1]): $($file.Name)") $file.FullName }
}
$planetLicense = Join-Path ([System.IO.Directory]::GetParent($projectRoot).FullName) 'LICENSE'
if (-not (Test-Path -LiteralPath $planetLicense -PathType Leaf)) { throw 'Planet LICENSE is missing.' }
[System.IO.Directory]::CreateDirectory($Destination) | Out-Null
[System.IO.File]::WriteAllText((Join-Path $Destination 'LICENSE-PLANET.txt'), [System.IO.File]::ReadAllText($planetLicense), $utf8)
[System.IO.File]::WriteAllText((Join-Path $Destination 'THIRD-PARTY-NOTICES.txt'), $notice.ToString(), $utf8)
[System.IO.File]::WriteAllText((Join-Path $Destination 'KUBO-BUILDINFO.txt'), (($kuboInfo -replace '^.*ipfs\.exe:', 'ipfs.exe:') -join "`n") + "`n", $utf8)
[System.IO.File]::WriteAllText((Join-Path $Destination 'KUBO-DEPENDENCY-LICENSES.txt'), [System.IO.File]::ReadAllText($kuboEvidence), $utf8)
Write-Host "Prepared Planet license and notices for $($modules.Count) Go modules and mygo-runtime $($runtimePackage.version)."
