# Runs on the Windows CI runner; all downloads use local fixtures.
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
$originalUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$originalPath = $env:Path
New-Item -ItemType Directory -Path $root | Out-Null
$script:corrupt = $false
function Invoke-RestMethod { param($Uri) return @{ tag_name = 'v1.2.3' } }
function Invoke-WebRequest {
    param($Uri, $OutFile, [switch]$UseBasicParsing)
    if ($Uri -notlike '*/releases/download/v1.2.3/*') { throw "Unexpected URL: $Uri" }
    if ($Uri.EndsWith('/checksums.txt')) {
        $hash = (Get-FileHash (Join-Path $root 'payload.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($script:corrupt) { $hash = '0' * 64 }
        [IO.File]::WriteAllText($OutFile, "$hash  axo-clash_windows_amd64.exe`n$hash  axo-clash_windows_arm64.exe`n")
    } else {
        Copy-Item (Join-Path $root 'payload.exe') $OutFile
    }
}
try {
    [IO.File]::WriteAllText((Join-Path $root 'payload.exe'), 'release fixture')
    $destination = Join-Path $root 'custom bin'
    & "$PSScriptRoot/../install.ps1" -InstallDir $destination
    $installed = Join-Path $destination 'axo-clash.exe'
    if ((Get-Content $installed -Raw) -ne 'release fixture') { throw 'Installation failed.' }
    if (($env:Path -split ';') -notcontains $destination) { throw 'PATH was not updated.' }
    $script:corrupt = $true
    [IO.File]::WriteAllText($installed, 'previous version')
    $rejected = $false
    try { & "$PSScriptRoot/../install.ps1" -Version v1.2.3 -InstallDir $destination }
    catch { $rejected = $true }
    if (-not $rejected) { throw 'Corrupted download accepted.' }
    if ((Get-Content $installed -Raw) -ne 'previous version') { throw 'Existing binary changed after failure.' }
    Write-Host 'PowerShell installer tests passed.'
} finally {
    [Environment]::SetEnvironmentVariable('Path', $originalUserPath, 'User')
    $env:Path = $originalPath
    Remove-Item $root -Recurse -Force
}
