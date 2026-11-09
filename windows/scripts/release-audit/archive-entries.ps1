function Assert-InstallerArchiveEntries {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Entries,
        [Parameter(Mandatory = $true)][string[]]$PayloadFiles,
        [Parameter(Mandatory = $true)][scriptblock]$GetEntryHash
    )

    $rootUninstaller = 'Uninstall.exe'
    $stagedUninstaller = '$PLUGINSDIR\update-payload\Uninstall.exe'
    $unexpectedEntries = @($Entries | Where-Object {
        -not $_.StartsWith('$PLUGINSDIR\') -and $_ -notin $PayloadFiles -and $_ -ne $rootUninstaller
    })
    if ($unexpectedEntries.Count) {
        throw ('Installer has unexpected payload entries; audit the changed payload and uninstall manifest: ' + ($unexpectedEntries | ConvertTo-Json -Compress))
    }

    # 7-Zip 24.09 hides both WriteUninstaller outputs; 26.05 exposes them.
    # A partially visible or duplicate pair is not either supported listing.
    $rootCount = @($Entries | Where-Object { $_ -eq $rootUninstaller }).Count
    $stagedCount = @($Entries | Where-Object { $_ -eq $stagedUninstaller }).Count
    if ($rootCount -eq 0 -and $stagedCount -eq 0) {
        return [pscustomobject]@{ status = 'not-listed'; sha256 = $null }
    }
    if ($rootCount -ne 1 -or $stagedCount -ne 1) {
        throw "Installer must have exactly one root and one staged Uninstall.exe when listed (root=$rootCount, staged=$stagedCount)."
    }

    $rootHash = & $GetEntryHash $rootUninstaller
    $stagedHash = & $GetEntryHash $stagedUninstaller
    if ($rootHash -ne $stagedHash) { throw 'Generated root and staged uninstallers differ.' }
    return [pscustomobject]@{ status = 'matched'; sha256 = $rootHash }
}
