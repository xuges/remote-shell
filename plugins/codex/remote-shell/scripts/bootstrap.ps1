# Download and verify the four CLI binaries for Windows.
param(
    [string]$Version = $(Get-Content -Raw "$PSScriptRoot\VERSION").Trim(),
    [string]$Prefix = $(if ($env:REMOTE_SHELL_PREFIX) { $env:REMOTE_SHELL_PREFIX } else { "$HOME\.remote-shell" })
)

$ErrorActionPreference = 'Stop'
$Commands = @('start-remote-shell', 'remote-shell', 'remote-shell-info', 'stop-remote-shell')
$Repo = 'xuges/remote-shell'
$InstallDir = Join-Path $Prefix 'bin'
Get-Command ssh -ErrorAction Stop | Out-Null

if ($Version -eq 'latest') {
    $Version = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
}
if ($Version -notmatch '^v[0-9][A-Za-z0-9._-]*$') { throw "Invalid release version: $Version" }

$ready = $true
foreach ($name in $Commands) {
    $exe = Join-Path $InstallDir "$name.exe"
    if (-not (Test-Path $exe -PathType Leaf)) { $ready = $false; break }
    if ((& $exe --version) -ne "$name $Version" -or $LASTEXITCODE -ne 0) { $ready = $false; break }
}
if ($ready) { Write-Host "[bootstrap] binaries ready: $InstallDir ($Version)"; return }

$nativeArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch ($nativeArch) {
    'ARM64' { $arch = 'arm64' }
    'AMD64' { $arch = 'amd64' }
    default { throw "Unsupported architecture: $nativeArch" }
}
$stem = "remote-shell-$Version-windows-$arch"
$base = if ($env:REMOTE_SHELL_DIST_BASE_URL) { $env:REMOTE_SHELL_DIST_BASE_URL }
        else { "https://github.com/$Repo/releases/download/$Version" }
$tmp = Join-Path ([IO.Path]::GetTempPath()) "rs-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $pkg = Join-Path $tmp "$stem.zip"
    $sums = Join-Path $tmp 'sha256sums.txt'
    Invoke-WebRequest "$base/$stem.zip" -OutFile $pkg
    Invoke-WebRequest "$base/sha256sums.txt" -OutFile $sums
    $matchesForAsset = @(foreach ($line in Get-Content $sums) {
        $parts = $line.Trim() -split '\s+'
        if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq "$stem.zip") { $parts[0] }
    })
    if ($matchesForAsset.Count -ne 1 -or $matchesForAsset[0] -notmatch '^[0-9a-fA-F]{64}$') {
        throw "Missing or ambiguous checksum for $stem.zip"
    }
    if ((Get-FileHash $pkg -Algorithm SHA256).Hash -ne $matchesForAsset[0]) {
        throw "Checksum mismatch for $stem.zip; nothing installed"
    }
    Expand-Archive -Path $pkg -DestinationPath $tmp
    foreach ($name in $Commands) {
        $exe = Join-Path (Join-Path $tmp $stem) "$name.exe"
        if (-not (Test-Path $exe -PathType Leaf)) { throw "Package missing executable: $name" }
        if ((& $exe --version) -ne "$name $Version" -or $LASTEXITCODE -ne 0) { throw "Unexpected executable version: $name" }
    }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    foreach ($name in $Commands) {
        Copy-Item (Join-Path (Join-Path $tmp $stem) "$name.exe") $InstallDir -Force
    }
    Write-Host "[bootstrap] installed $Version into $InstallDir"
    Write-Host '[bootstrap] Use the absolute executable paths, or add the bin directory to PATH for this session.'
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
