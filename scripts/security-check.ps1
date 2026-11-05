# Run online advisory lookups; this script does not dispatch GitHub Actions.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskFailures = @()
Push-Location (Join-Path $taskRoot 'windows')
try {
    Write-Host 'Checking Bun dependencies against published advisories...'
    & bun audit
    if ($LASTEXITCODE -ne 0) { $taskFailures += 'Bun dependency audit' }
    Write-Host 'Checking reachable Go vulnerabilities with govulncheck v1.9.0...'
    & go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
    if ($LASTEXITCODE -ne 0) { $taskFailures += 'Go vulnerability check' }
    if ($taskFailures.Count -gt 0) {
        throw "Security checks did not pass: $($taskFailures -join ', '). Review findings or network/tool errors above."
    }
    Write-Host 'Dependency security checks passed. Bundled Kubo and template assets need separate review.'
} finally {
    Pop-Location
}
