[CmdletBinding()]
param(
    [string]$ArchivePath = ''
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$releaseTag = 'b10621'
$assetName = 'llama-b10621-bin-win-cpu-x64.zip'
$expectedSHA256 = '0e8b65e650e369f70f8307d890508886f171ef4fb00facccddd4a1b7ffdaca51'
$assetURL = "https://github.com/ggml-org/llama.cpp/releases/download/$releaseTag/$assetName"
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$cacheRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot ".build\llama-cpp\$releaseTag"))
$destination = [IO.Path]::GetFullPath((Join-Path $projectRoot 'dist\tools\llama'))

foreach ($ownedPath in @($cacheRoot, $destination)) {
    if (-not $ownedPath.StartsWith($projectRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the repository: $ownedPath"
    }
}
New-Item -ItemType Directory -Path $cacheRoot -Force | Out-Null

if ($ArchivePath) {
    $archive = [IO.Path]::GetFullPath($ArchivePath)
    if (-not (Test-Path -LiteralPath $archive)) {
        throw "llama.cpp archive not found: $archive"
    }
} else {
    $archive = Join-Path $cacheRoot $assetName
    if (-not (Test-Path -LiteralPath $archive)) {
        $partial = $archive + '.part'
        Write-Host "Downloading llama.cpp $releaseTag..." -ForegroundColor Cyan
        Invoke-WebRequest -UseBasicParsing -Headers @{ 'User-Agent' = 'DownKit-build' } -Uri $assetURL -OutFile $partial
        Move-Item -LiteralPath $partial -Destination $archive -Force
    }
}

$actualSHA256 = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualSHA256 -ne $expectedSHA256) {
    throw "llama.cpp archive SHA-256 mismatch: expected=$expectedSHA256 actual=$actualSHA256"
}

$nonce = [Guid]::NewGuid().ToString('N')
$extractRoot = Join-Path $cacheRoot "extract-$nonce"
$bundleRoot = Join-Path $cacheRoot "bundle-$nonce"
$backup = $destination + ".previous-$nonce"
New-Item -ItemType Directory -Path $extractRoot -Force | Out-Null
New-Item -ItemType Directory -Path $bundleRoot -Force | Out-Null

try {
    Expand-Archive -LiteralPath $archive -DestinationPath $extractRoot
    foreach ($name in @('llama-server.exe', 'llama-server-impl.dll', 'llama-common.dll', 'llama.dll', 'ggml.dll', 'ggml-base.dll', 'libomp.dll', 'mtmd.dll', 'LICENSE-LLVM-OpenMP')) {
        $source = Join-Path $extractRoot $name
        if (-not (Test-Path -LiteralPath $source)) {
            throw "Official llama.cpp archive is missing $name"
        }
        Copy-Item -LiteralPath $source -Destination $bundleRoot
    }
    $cpuBackends = @(Get-ChildItem -LiteralPath $extractRoot -Filter 'ggml-cpu-*.dll' -File)
    if ($cpuBackends.Count -eq 0) {
        throw 'Official llama.cpp archive does not contain CPU runtime backends'
    }
    $cpuBackends | Copy-Item -Destination $bundleRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'LICENSE') -Destination $bundleRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'SOURCE.txt') -Destination $bundleRoot

    $stagedServer = Join-Path $bundleRoot 'llama-server.exe'
    & $stagedServer --version
    if ($LASTEXITCODE -ne 0) {
        throw "Staged llama-server validation failed with exit code $LASTEXITCODE"
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

Write-Host "llama-server ready: $destination" -ForegroundColor Green
