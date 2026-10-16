# Usage: irm https://raw.githubusercontent.com/axo-clash/cli/HEAD/install.ps1 | iex
param(
    [string]$Version = 'latest',
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\axo-clash')
)

& {
    $ErrorActionPreference = 'Stop'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $repo = 'https://github.com/axo-clash/cli'
    if ($env:OS -ne 'Windows_NT') { throw 'Ce script est destiné à Windows.' }
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($architecture) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "Architecture non supportée : $architecture" }
    }
    if ($Version -eq 'latest') {
        $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/axo-clash/cli/releases/latest'
        $Version = $release.tag_name
    }
    if ($Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$') {
        throw 'Version invalide (ex. v1.2.3).'
    }
    $asset = "axo-clash_windows_$arch.exe"
    $temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $temp | Out-Null
    try {
        $download = Join-Path $temp $asset
        Invoke-WebRequest -UseBasicParsing -Uri "$repo/releases/download/$Version/$asset" -OutFile $download
        $checksumFile = Join-Path $temp 'checksums.txt'
        Invoke-WebRequest -UseBasicParsing -Uri "$repo/releases/download/$Version/checksums.txt" -OutFile $checksumFile
        $entries = @(Get-Content -LiteralPath $checksumFile | Where-Object { $_ -cmatch "^[0-9a-f]{64}  $([regex]::Escape($asset))$" })
        if ($entries.Count -ne 1) { throw 'Somme SHA-256 absente ou invalide.' }
        $expected = $entries[0].Substring(0, 64)
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $download).Hash
        if ($actual -ine $expected) { throw 'Échec de la vérification SHA-256.' }
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        Move-Item -LiteralPath $download -Destination (Join-Path $InstallDir 'axo-clash.exe') -Force
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (($userPath -split ';') -notcontains $InstallDir) {
            [Environment]::SetEnvironmentVariable('Path', "$InstallDir;$userPath", 'User')
        }
        if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$InstallDir;$env:Path" }
        Write-Host "AXO Clash $Version installé dans $InstallDir."
    }
    finally {
        Remove-Item -LiteralPath $temp -Recurse -Force
    }
}
