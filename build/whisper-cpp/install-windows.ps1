[CmdletBinding()]
param(
    [string]$ArchivePath = ''
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$releaseTag = 'b4938'
$assetName = 'whisper-bin-x64.zip'
$expectedSHA256 = 'c2a4b60edb11f7e11a9191ffb50929535527d4d91c9903dbe3e554583bbbc63d'
$assetURL = "https://github.com/ggml-org/whisper.cpp/releases/download/$releaseTag/$assetName"
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$cacheRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot ".build\whisper-cpp\$releaseTag"))
$destination = [IO.Path]::GetFullPath((Join-Path $projectRoot 'dist\tools\whisper'))

foreach ($ownedPath in @($cacheRoot, $destination)) {
    if (-not $ownedPath.StartsWith($projectRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the repository: $ownedPath"
    }
}
New-Item -ItemType Directory -Path $cacheRoot -Force | Out-Null

if ($ArchivePath) {
    $archive = [IO.Path]::GetFullPath($ArchivePath)
    if (-not (Test-Path -LiteralPath $archive)) {
        throw "Whisper archive not found: $archive"
    }
} else {
    $archive = Join-Path $cacheRoot $assetName
    if (-not (Test-Path -LiteralPath $archive)) {
        $partial = $archive + '.part'
        Write-Host "Downloading whisper.cpp $releaseTag..." -ForegroundColor Cyan
        Invoke-WebRequest -UseBasicParsing -Headers @{ 'User-Agent' = 'DownKit-build' } -Uri $assetURL -OutFile $partial
        Move-Item -LiteralPath $partial -Destination $archive -Force
    }
}

$actualSHA256 = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualSHA256 -ne $expectedSHA256) {
    throw "Whisper archive SHA-256 mismatch: expected=$expectedSHA256 actual=$actualSHA256"
}

$nonce = [Guid]::NewGuid().ToString('N')
$extractRoot = Join-Path $cacheRoot "extract-$nonce"
$bundleRoot = Join-Path $cacheRoot "bundle-$nonce"
$backup = $destination + ".previous-$nonce"
New-Item -ItemType Directory -Path $extractRoot -Force | Out-Null
New-Item -ItemType Directory -Path $bundleRoot -Force | Out-Null

try {
    Expand-Archive -LiteralPath $archive -DestinationPath $extractRoot
    $releaseRoot = Join-Path $extractRoot 'Release'
    $requiredFiles = @('whisper-cli.exe', 'whisper.dll', 'ggml.dll', 'ggml-base.dll')
    foreach ($name in $requiredFiles) {
        $source = Join-Path $releaseRoot $name
        if (-not (Test-Path -LiteralPath $source)) {
            throw "Official Whisper archive is missing $name"
        }
        Copy-Item -LiteralPath $source -Destination $bundleRoot
    }
    $cpuBackends = @(Get-ChildItem -LiteralPath $releaseRoot -Filter 'ggml-cpu-*.dll' -File)
    if ($cpuBackends.Count -eq 0) {
        throw 'Official Whisper archive does not contain CPU runtime backends'
    }
    $cpuBackends | Copy-Item -Destination $bundleRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'LICENSE') -Destination $bundleRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'SOURCE.txt') -Destination $bundleRoot

    $stagedClient = Join-Path $bundleRoot 'whisper-cli.exe'
    & $stagedClient --version
    if ($LASTEXITCODE -ne 0) {
        throw "Staged whisper-cli validation failed with exit code $LASTEXITCODE"
    }

    New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
    if (Test-Path -LiteralPath $destination) {
        Move-Item -LiteralPath $destination -Destination $backup
    }
    try {
        Move-Item -LiteralPath $bundleRoot -Destination $destination
    } catch {
        if (Test-Path -LiteralPath $backup) {
            Move-Item -LiteralPath $backup -Destination $destination
        }
        throw
    }
    if (Test-Path -LiteralPath $backup) {
        Remove-Item -LiteralPath $backup -Recurse -Force
    }
} finally {
    foreach ($temporary in @($extractRoot, $bundleRoot)) {
        $resolved = [IO.Path]::GetFullPath($temporary)
        if ($resolved.StartsWith($cacheRoot, [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $resolved)) {
            Remove-Item -LiteralPath $resolved -Recurse -Force
        }
    }
}

Write-Host "Whisper Client ready: $destination" -ForegroundColor Green
