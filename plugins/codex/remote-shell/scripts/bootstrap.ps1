# remote-shell PowerShell installer for Windows hosts.
# Downloads prebuilt release binaries, verifies sha256, installs to
# ~/.remote-shell\bin. No Go toolchain required.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File bootstrap.ps1 [[-Version] v1.0.0|latest]
param(
    [string]$Version = $(Get-Content -Raw "$PSScriptRoot\VERSION").Trim(),
    [string]$Prefix = "$HOME\.remote-shell"
)

$ErrorActionPreference = 'Stop'
$Commands = @('start-remote-shell', 'remote-shell', 'remote-shell-info', 'stop-remote-shell')
$Repo = 'xuges/remote-shell'
$Installdir = Join-Path $Prefix 'bin'

function TestSha256([string]$file, [string]$sumsFile) {
    $expected = (Get-Content $sumsFile | Where-Object { $_ -match [regex]::Escape((Split-Path $file -Leaf)) }) -split '\s+' | Select-Object -First 1
    if (-not $expected) { return $false }
    $actual = (Get-FileHash $file -Algorithm SHA256).Hash.ToLowerInvariant()
    return $actual -eq $expected.ToLowerInvariant()
}

if ($Version -eq 'latest') {
    $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $releases.tag_name
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$stem = "remote-shell-$Version-windows-$arch"
$base = if ($env:REMOTE_SHELL_DIST_BASE_URL) { $env:REMOTE_SHELL_DIST_BASE_URL }
        else { "https://github.com/$Repo/releases/download/$Version" }

New-Item -ItemType Directory -Force -Path $Installdir | Out-Null
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "rs-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
    $pkg = Join-Path $tmp "$stem.zip"
    $sums = Join-Path $tmp 'sha256sums.txt'
    Invoke-WebRequest "$base/$stem.zip" -OutFile $pkg
    Invoke-WebRequest "$base/sha256sums.txt" -OutFile $sums
    if (-not (TestSha256 $pkg $sums)) { throw "checksum mismatch for $stem.zip — refusing to install" }
    Expand-Archive -Path $pkg -DestinationPath $tmp
    Get-ChildItem (Join-Path $tmp $stem) -Filter '*.exe' | ForEach-Object {
        Copy-Item $_.FullName $Installdir -Force
    }
    Write-Host "[bootstrap] installed $Version into $Installdir"
    $inPath = ($env:PATH -split ';') -contains $Installdir
    if (-not $inPath) {
        Write-Host "[bootstrap] add to your user PATH: setx PATH \"$Installdir;$env:PATH\""
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}