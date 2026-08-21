[CmdletBinding()]
param(
    [string]$FFmpegTag = 'n8.1.2',
    [string]$OutputDir = '',
    [string]$NetworkProxy = ''
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $OutputDir) {
    $OutputDir = Join-Path $projectRoot 'dist\tools'
}
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$buildRoot = Join-Path $projectRoot '.build\ffmpeg-slim-windows'
$sourceDir = Join-Path $buildRoot 'ffmpeg'
$archivePath = Join-Path $buildRoot "FFmpeg-$FFmpegTag.tar.gz"

$bashCandidates = @(
    'C:\msys64\usr\bin\bash.exe',
    (Join-Path $env:LOCALAPPDATA 'Programs\msys64\usr\bin\bash.exe')
)
$bash = $bashCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $bash) {
    throw 'MSYS2 is required. Install it with: winget install --id MSYS2.MSYS2 --exact'
}

New-Item -ItemType Directory -Path $buildRoot,$OutputDir -Force | Out-Null

if ($NetworkProxy) {
    $env:HTTP_PROXY = $NetworkProxy
    $env:HTTPS_PROXY = $NetworkProxy
    $env:http_proxy = $NetworkProxy
    $env:https_proxy = $NetworkProxy
}

& $bash -lc 'pacman -S --needed --noconfirm make diffutils curl pkgconf mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-nasm'
if ($LASTEXITCODE -ne 0) {
    throw "Could not install the MSYS2 build dependencies: $LASTEXITCODE"
}

$env:CHERE_INVOKING = '1'
$env:MSYSTEM = 'UCRT64'
function Convert-ToMSYSPath([string]$Path) {
    $converted = & $bash -lc 'cygpath -u "$1"' _ $Path
    if ($LASTEXITCODE -ne 0) {
        throw "Could not convert this path for MSYS2: $Path"
    }
    return $converted.Trim()
}

if (-not (Test-Path -LiteralPath (Join-Path $sourceDir 'configure'))) {
    if (-not (Test-Path -LiteralPath $archivePath)) {
        $archivePart = "$archivePath.part"
        $archivePartUnix = Convert-ToMSYSPath $archivePart
        $sourceURL = "https://github.com/FFmpeg/FFmpeg/archive/refs/tags/$FFmpegTag.tar.gz"
        & $bash -lc 'curl --fail --location --retry 3 --output "$1" "$2"' _ $archivePartUnix $sourceURL
        if ($LASTEXITCODE -ne 0) {
            throw "Could not download FFmpeg source: $LASTEXITCODE"
        }
        Move-Item -LiteralPath $archivePart -Destination $archivePath -Force
    }
    if (Test-Path -LiteralPath $sourceDir) {
        Remove-Item -LiteralPath $sourceDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $sourceDir -Force | Out-Null
    & tar.exe -xzf $archivePath --strip-components=1 -C $sourceDir
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath (Join-Path $sourceDir 'configure'))) {
        throw "Could not extract FFmpeg source: $LASTEXITCODE"
    }
}

$sourceUnix = Convert-ToMSYSPath $sourceDir
$outputUnix = Convert-ToMSYSPath $OutputDir
$buildUnix = Convert-ToMSYSPath $buildRoot
$scriptUnix = Convert-ToMSYSPath (Join-Path $PSScriptRoot 'build-native.sh')

& $bash -lc "export PATH=/ucrt64/bin:/usr/bin; bash '$scriptUnix' '$sourceUnix' '$buildUnix/native' '$outputUnix'"
if ($LASTEXITCODE -ne 0) {
    throw "FFmpeg Slim build failed: $LASTEXITCODE"
}

$ffmpeg = Join-Path $OutputDir 'ffmpeg-slim.exe'
if (-not (Test-Path -LiteralPath $ffmpeg)) {
    throw "Expected output was not created: $ffmpeg"
}

& $ffmpeg -hide_banner -version | Select-Object -First 1
$sizeMiB = [math]::Round((Get-Item -LiteralPath $ffmpeg).Length / 1MB, 2)
Write-Host "Done: $ffmpeg ($sizeMiB MiB)" -ForegroundColor Green
