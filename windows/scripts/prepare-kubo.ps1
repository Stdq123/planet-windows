$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Directory]::GetParent($PSScriptRoot).FullName
$cache = [System.IO.Path]::Combine($projectRoot, '.mygo', 'kubo')
$archive = [System.IO.Path]::Combine($cache, 'kubo.zip')
$destination = [System.IO.Path]::Combine($projectRoot, 'resources', 'windows-amd64')
$expected = '8ada6ced4c11eb7b2f7ef3c38f2ce596efbcf411c5a0ba6c1e50f6c59f81fcc0881a0325e3add6c4462a71a477b2f02a931ab24bc7d43a98b42e402e603a4f2f'
[System.IO.Directory]::CreateDirectory($cache) | Out-Null
[System.IO.Directory]::CreateDirectory($destination) | Out-Null
if (-not [System.IO.File]::Exists($archive)) {
    & curl.exe -fL --retry 2 -o $archive 'https://github.com/ipfs/kubo/releases/download/v0.38.2/kubo_v0.38.2_windows-amd64.zip'
    if ($LASTEXITCODE -ne 0) { throw 'Kubo download failed.' }
}
$stream = [System.IO.File]::OpenRead($archive)
try { $sha = [System.Security.Cryptography.SHA512]::Create(); try { $digest = $sha.ComputeHash($stream) } finally { $sha.Dispose() } } finally { $stream.Dispose() }
$actual = [System.BitConverter]::ToString($digest).Replace('-', '').ToLowerInvariant()
if ($actual -ne $expected) { throw 'Kubo archive checksum mismatch.' }
[System.Reflection.Assembly]::Load('System.IO.Compression.FileSystem, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089') | Out-Null
$zip = [System.IO.Compression.ZipFile]::OpenRead($archive)
try {
    foreach ($name in @('ipfs.exe', 'LICENSE', 'LICENSE-APACHE', 'LICENSE-MIT')) {
        $entry = $zip.GetEntry("kubo/$name")
        if ($null -eq $entry) { throw "Missing Kubo archive entry: $name" }
        $targetName = if ($name -eq 'ipfs.exe') { $name } else { "kubo-$name.txt" }
        $target = [System.IO.Path]::Combine($destination, $targetName)
        $inputStream = $entry.Open()
        try { $outputStream = [System.IO.File]::Create($target); try { $inputStream.CopyTo($outputStream) } finally { $outputStream.Dispose() } } finally { $inputStream.Dispose() }
    }
} finally { $zip.Dispose() }
