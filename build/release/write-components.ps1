[CmdletBinding()]
param(
    [string]$DistDir = '',
    [string]$Platform = 'windows',
    [string]$Architecture = 'amd64'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..')
if (-not $DistDir) { $DistDir = Join-Path $projectRoot 'dist' }
$DistDir = [IO.Path]::GetFullPath($DistDir)

$versionSource = Get-Content -LiteralPath (Join-Path $projectRoot 'bridge\bridge_config.go') -Raw
$versionMatch = [regex]::Match($versionSource, 'const bridgeVersion = "([^"]+)"')
if (-not $versionMatch.Success) { throw 'Could not read bridgeVersion' }

function Artifact([string]$relativePath) {
    $path = Join-Path $DistDir $relativePath
    if (-not (Test-Path -LiteralPath $path)) { return $null }
    $file = Get-Item -LiteralPath $path
    return [ordered]@{
        path = $relativePath.Replace('\', '/')
        size = $file.Length
        sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    }
}

function Artifacts([string]$relativeDirectory) {
    $directory = Join-Path $DistDir $relativeDirectory
    if (-not (Test-Path -LiteralPath $directory)) { return @() }
    return @(
        Get-ChildItem -LiteralPath $directory -File |
            Sort-Object Name |
            ForEach-Object { Artifact (Join-Path $relativeDirectory $_.Name) }
    )
}

$ffmpegName = if ($Platform -eq 'windows') { 'tools\ffmpeg-slim.exe' } else { 'tools/ffmpeg-slim' }
$whisperName = if ($Platform -eq 'windows') { 'tools\whisper\whisper-cli.exe' } else { 'tools/whisper/whisper-cli' }
$whisperArtifact = Artifact $whisperName
$llamaName = if ($Platform -eq 'windows') { 'tools\llama\llama-server.exe' } else { 'tools/llama/llama-server' }
$llamaArtifact = Artifact $llamaName
$manifest = [ordered]@{
    schemaVersion = 1
    productVersion = $versionMatch.Groups[1].Value
    platform = $Platform
    architecture = $Architecture
    generatedAt = [DateTime]::UtcNow.ToString('o')
    components = @(
        [ordered]@{
            id = 'ffmpeg'
            displayName = 'FFmpeg Slim'
            delivery = 'bundled-sidecar'
            requiredFor = @('media.remux', 'media.merge')
            artifact = Artifact $ffmpegName
            license = 'LGPL-2.1-or-later'
            licenseFile = 'tools/COPYING.LGPLv2.1'
            sourceFile = 'tools/ffmpeg-slim-SOURCE.txt'
        },
        [ordered]@{
            id = 'yt-dlp'
            displayName = 'yt-dlp'
            delivery = 'on-demand'
            requiredFor = @('page.resolve', 'playlist.resolve')
            releaseBaseUrl = 'https://github.com/yt-dlp/yt-dlp/releases/latest/download'
            checksumAsset = 'SHA2-256SUMS'
            licenseUrl = 'https://github.com/yt-dlp/yt-dlp/blob/master/README.md#licensing'
        },
        [ordered]@{
            id = 'whisper.cpp'
            displayName = 'whisper.cpp'
            delivery = if ($whisperArtifact) { 'bundled-sidecar' } else { 'external' }
            requiredFor = @('subtitle.asr', 'audio.transcribe')
            artifact = $whisperArtifact
            bundleArtifacts = @(Artifacts 'tools\whisper')
            license = 'MIT'
            licenseFile = 'tools/whisper/LICENSE'
            sourceFile = 'tools/whisper/SOURCE.txt'
        },
        [ordered]@{
            id = 'llama.cpp'
            displayName = 'llama.cpp'
            delivery = if ($llamaArtifact) { 'bundled-sidecar' } else { 'external' }
            requiredFor = @('subtitle.translate')
            artifact = $llamaArtifact
            bundleArtifacts = @(Artifacts 'tools\llama')
            license = 'MIT'
            licenseFile = 'tools/llama/LICENSE'
            sourceFile = 'tools/llama/SOURCE.txt'
        }
    )
}

$target = Join-Path $DistDir 'components.json'
$json = $manifest | ConvertTo-Json -Depth 10
[IO.File]::WriteAllText($target, $json + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
Write-Host "Component manifest: $target" -ForegroundColor Green
