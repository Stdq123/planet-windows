# Run from any directory; use the same checks locally and in GitHub Actions.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$taskRepositoryRoot = Split-Path -Parent $PSScriptRoot

function Invoke-Check {
    param([string]$Label, [scriptblock]$Action)
    Write-Host "`n== $Label =="
    & $Action
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed (exit code $LASTEXITCODE)."
    }
}

Push-Location (Join-Path $taskRepositoryRoot 'windows')
try {
    foreach ($taskCommand in @('git', 'go', 'gofmt', 'bun')) {
        if (-not (Get-Command $taskCommand -ErrorAction SilentlyContinue)) {
            throw "Required command not found: $taskCommand. See README.md for setup."
        }
    }

    Invoke-Check 'Documentation' { bun run check:docs }

    Write-Host "`n== Go formatting (read-only) =="
    $taskGoFiles = @(git -c core.quotepath=false ls-files --cached --others --exclude-standard -- '*.go')
    if ($LASTEXITCODE -ne 0) { throw 'Cannot list Go source files.' }
    # Deleted tracked files are still in the index until staged.
    $taskGoFiles = @($taskGoFiles | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf })
    if ($taskGoFiles.Count -eq 0) { throw 'No Go source files found.' }
    $taskUnformatted = @(& gofmt -l $taskGoFiles)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt could not read the Go sources.' }
    if ($taskUnformatted.Count -gt 0) {
        throw "Run gofmt -w on these files:`n$($taskUnformatted -join "`n")"
    }

    Invoke-Check 'Go static analysis' { go vet ./... }
    Invoke-Check 'Frontend tests' { bun test }
    Invoke-Check 'Go tests (uncached)' { go test -count=1 -timeout 5m ./... }
    Invoke-Check 'TypeScript and frontend production build' { bun run build:web }
    Write-Host "`nAll checks passed. Installer and optional integration tests are separate."
} finally {
    Pop-Location
}
