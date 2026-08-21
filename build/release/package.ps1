[CmdletBinding()]
param(
    [string]$Version = '',
    [string]$FFmpegSourceArchive = ''
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$extensionManifestPath = Join-Path $projectRoot 'extension\manifest.json'
$bridgeVersionPath = Join-Path $projectRoot 'bridge\bridge_config.go'
$extensionVersion = (Get-Content -LiteralPath $extensionManifestPath -Raw | ConvertFrom-Json).version
$bridgeVersionSource = Get-Content -LiteralPath $bridgeVersionPath -Raw
$bridgeVersionMatch = [regex]::Match($bridgeVersionSource, 'const bridgeVersion = "([^"]+)"')
if (-not $bridgeVersionMatch.Success) {
    throw 'Could not read bridgeVersion'
}
$bridgeVersion = $bridgeVersionMatch.Groups[1].Value
if (-not $Version) {
    $Version = $extensionVersion
}
$Version = $Version.TrimStart('v')
if ($Version -ne $extensionVersion -or $Version -ne $bridgeVersion) {
    throw "Release version mismatch: requested=$Version, extension=$extensionVersion, bridge=$bridgeVersion"
}

$buildRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot '.build\release'))
$outputRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot "dist\release\v$Version"))
foreach ($target in @($buildRoot, $outputRoot)) {
    if (-not $target.StartsWith($projectRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the repository: $target"
    }
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Recurse -Force
    }
    New-Item -ItemType Directory -Path $target -Force | Out-Null
}

$commonFiles = @('README.md', 'LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md')
$gitBin = Split-Path -Parent (Get-Command git -ErrorAction Stop).Source
$gitRootCandidates = @(
    (Split-Path -Parent $gitBin),
    (Split-Path -Parent (Split-Path -Parent $gitBin))
)
$gitRoot = $gitRootCandidates |
    Where-Object { Test-Path -LiteralPath (Join-Path $_ 'usr\bin\tar.exe') } |
    Select-Object -First 1
if (-not $gitRoot) {
    throw 'GNU tar and gzip from Git for Windows are required to preserve Unix executable permissions'
}
$gnuTar = Join-Path $gitRoot 'usr\bin\tar.exe'
$gzip = Join-Path $gitRoot 'usr\bin\gzip.exe'
if (-not (Test-Path -LiteralPath $gzip)) {
    throw 'GNU gzip from Git for Windows is required to create Unix release archives'
}

function Convert-ToMSYSPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    $fullPath = [IO.Path]::GetFullPath($Path)
    if ($fullPath -notmatch '^([A-Za-z]):\\(.*)$') {
        throw "Cannot convert path for Git for Windows: $fullPath"
    }
    return '/' + $Matches[1].ToLowerInvariant() + '/' + $Matches[2].Replace('\', '/')
}
$targets = @(
    [ordered]@{ Platform = 'windows'; GoOS = 'windows'; Architecture = 'amd64'; Extension = '.exe'; Archive = 'zip' },
    [ordered]@{ Platform = 'linux'; GoOS = 'linux'; Architecture = 'amd64'; Extension = ''; Archive = 'tar.gz' },
    [ordered]@{ Platform = 'linux'; GoOS = 'linux'; Architecture = 'arm64'; Extension = ''; Archive = 'tar.gz' },
    [ordered]@{ Platform = 'macos'; GoOS = 'darwin'; Architecture = 'amd64'; Extension = ''; Archive = 'tar.gz' },
    [ordered]@{ Platform = 'macos'; GoOS = 'darwin'; Architecture = 'arm64'; Extension = ''; Archive = 'tar.gz' }
)

$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED
$previousGoCache = $env:GOCACHE
$env:CGO_ENABLED = '0'
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build-release'
New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null

try {
    foreach ($target in $targets) {
        $platform = $target.Platform
        $architecture = $target.Architecture
        $packageName = "downkit-v$Version-$platform-$architecture"
        $packageDir = Join-Path $buildRoot $packageName
        New-Item -ItemType Directory -Path $packageDir -Force | Out-Null

        $binaryName = "downkit-$platform-$architecture$($target.Extension)"
        $binaryPath = Join-Path $packageDir $binaryName
        $env:GOOS = $target.GoOS
        $env:GOARCH = $architecture
        Write-Host "Building $platform/$architecture..." -ForegroundColor Cyan
        Push-Location (Join-Path $projectRoot 'bridge')
        try {
            & go build -buildvcs=false -trimpath -ldflags '-s -w' -o $binaryPath .\cmd\downkit
            if ($LASTEXITCODE -ne 0) {
                throw "Bridge build failed for $platform/$architecture with exit code $LASTEXITCODE"
            }
        } finally {
            Pop-Location
        }

        foreach ($file in $commonFiles) {
            Copy-Item -LiteralPath (Join-Path $projectRoot $file) -Destination $packageDir
        }
        Copy-Item -LiteralPath (Join-Path $projectRoot 'extension') -Destination $packageDir -Recurse
        $extensionTests = Join-Path $packageDir 'extension\test'
        if (Test-Path -LiteralPath $extensionTests) {
            Remove-Item -LiteralPath $extensionTests -Recurse -Force
        }

        if ($platform -eq 'windows') {
            foreach ($file in @('Install-Windows.ps1', 'Uninstall-Windows.ps1', '安装-Windows.cmd', '卸载-Windows.cmd')) {
                Copy-Item -LiteralPath (Join-Path $projectRoot $file) -Destination $packageDir
            }
            $toolsDir = Join-Path $packageDir 'tools'
            New-Item -ItemType Directory -Path $toolsDir -Force | Out-Null
            Copy-Item -LiteralPath (Join-Path $projectRoot 'dist\tools\ffmpeg-slim.exe') -Destination $toolsDir
            Copy-Item -LiteralPath (Join-Path $projectRoot 'dist\tools\COPYING.LGPLv2.1') -Destination $toolsDir
            Copy-Item -LiteralPath (Join-Path $projectRoot 'dist\tools\ffmpeg-slim-SOURCE.txt') -Destination $toolsDir
            Copy-Item -LiteralPath (Join-Path $projectRoot 'build\ffmpeg-slim\build-windows.ps1') -Destination (Join-Path $toolsDir 'ffmpeg-slim-BUILD.ps1')
            & (Join-Path $projectRoot 'build\release\write-components.ps1') -DistDir $packageDir -Platform windows -Architecture $architecture
        } elseif ($platform -eq 'linux') {
            Copy-Item -LiteralPath (Join-Path $projectRoot 'Install-Linux.sh') -Destination $packageDir
            Copy-Item -LiteralPath (Join-Path $projectRoot 'Uninstall-Linux.sh') -Destination $packageDir
            $unixExecutables = @($binaryName, 'Install-Linux.sh', 'Uninstall-Linux.sh')
        } else {
            Copy-Item -LiteralPath (Join-Path $projectRoot 'Install-macOS.command') -Destination $packageDir
            Copy-Item -LiteralPath (Join-Path $projectRoot 'Uninstall-macOS.command') -Destination $packageDir
            $unixExecutables = @($binaryName, 'Install-macOS.command', 'Uninstall-macOS.command')
        }

        $archivePath = Join-Path $outputRoot "$packageName.$($target.Archive)"
        if ($target.Archive -eq 'zip') {
            Compress-Archive -LiteralPath $packageDir -DestinationPath $archivePath -CompressionLevel Optimal
        } else {
            $tarPath = Join-Path $outputRoot "$packageName.tar"
            $tarPathUnix = Convert-ToMSYSPath $tarPath
            $buildRootUnix = Convert-ToMSYSPath $buildRoot
            & $gnuTar --format=ustar --owner=0 --group=0 --numeric-owner '--mode=u+rwX,go+rX' -cf $tarPathUnix -C $buildRootUnix $packageName
            if ($LASTEXITCODE -ne 0) {
                throw "Could not create $tarPath"
            }
            $executableMembers = @($unixExecutables | ForEach-Object { "$packageName/$_" })
            & $gnuTar --delete -f $tarPathUnix @executableMembers
            if ($LASTEXITCODE -ne 0) {
                throw "Could not update executable entries in $tarPath"
            }
            foreach ($member in $executableMembers) {
                & $gnuTar --format=ustar --owner=0 --group=0 --numeric-owner '--mode=755' -rf $tarPathUnix -C $buildRootUnix $member
                if ($LASTEXITCODE -ne 0) {
                    throw "Could not mark $member executable in $tarPath"
                }
            }
            & $gzip -9 -n -f $tarPathUnix
            if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $archivePath)) {
                throw "Could not create $archivePath"
            }
        }
    }
} finally {
    $env:GOOS = $previousGoOS
    $env:GOARCH = $previousGoArch
    $env:CGO_ENABLED = $previousCgoEnabled
    $env:GOCACHE = $previousGoCache
}

if ($FFmpegSourceArchive) {
    $sourcePath = [IO.Path]::GetFullPath($FFmpegSourceArchive)
    if (-not (Test-Path -LiteralPath $sourcePath)) {
        throw "FFmpeg source archive not found: $sourcePath"
    }
    $expectedSourceHash = '9fd092511605bbebafe095ea6d38d9e40f34d12f7386e1258372df8be0576eb7'
    $actualSourceHash = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualSourceHash -ne $expectedSourceHash) {
        throw "FFmpeg source archive hash mismatch: expected=$expectedSourceHash actual=$actualSourceHash"
    }
    Copy-Item -LiteralPath $sourcePath -Destination (Join-Path $outputRoot 'FFmpeg-n8.1.2-source.tar.gz')
}

$checksumLines = Get-ChildItem -LiteralPath $outputRoot -File |
    Where-Object Name -NE 'SHA256SUMS.txt' |
    Sort-Object Name |
    ForEach-Object {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $_.Name
    }
$checksumPath = Join-Path $outputRoot 'SHA256SUMS.txt'
[IO.File]::WriteAllLines($checksumPath, $checksumLines, [Text.UTF8Encoding]::new($false))

Write-Host "Release assets: $outputRoot" -ForegroundColor Green
Get-ChildItem -LiteralPath $outputRoot -File |
    Sort-Object Name |
    Select-Object Name, Length |
    Format-Table -AutoSize
