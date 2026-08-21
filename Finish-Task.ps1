[CmdletBinding()]
param(
    [switch]$SkipTests
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$projectRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$bridgeDir = Join-Path $projectRoot 'bridge'
$extensionTests = Join-Path $projectRoot 'extension\test'
$distDir = Join-Path $projectRoot 'dist'
$target = Join-Path $distDir 'downkit-windows-amd64.exe'
$staged = Join-Path $distDir 'downkit-windows-amd64.pending.exe'
$goCache = Join-Path $projectRoot '.cache\go-build-finish-task'

New-Item -ItemType Directory -Force -Path $distDir, $goCache | Out-Null
$env:GOCACHE = $goCache
$env:CGO_ENABLED = '0'

function Get-Sha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$LiteralPath
    )

    $algorithm = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($LiteralPath)
    try {
        return ([BitConverter]::ToString($algorithm.ComputeHash($stream))).Replace('-', '')
    } finally {
        $stream.Dispose()
        $algorithm.Dispose()
    }
}

if (-not $SkipTests) {
    Write-Host 'Running Go tests...' -ForegroundColor Cyan
    Push-Location $bridgeDir
    try {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Write-Host 'Running extension tests...' -ForegroundColor Cyan
    Get-ChildItem -LiteralPath $extensionTests -Filter '*.test.js' -File |
        Sort-Object Name |
        ForEach-Object {
            & node $_.FullName
            if ($LASTEXITCODE -ne 0) { throw "Extension test failed: $($_.Name)" }
        }
}

Write-Host 'Building Bridge...' -ForegroundColor Cyan
if (Test-Path -LiteralPath $staged) {
    Remove-Item -LiteralPath $staged -Force
}
Push-Location $bridgeDir
try {
    & go build -buildvcs=false -trimpath -o $staged .\cmd\downkit
    if ($LASTEXITCODE -ne 0) { throw "Bridge build failed with exit code $LASTEXITCODE" }
} finally {
    Pop-Location
}

try {
    $stagedHash = Get-Sha256 -LiteralPath $staged
    Copy-Item -LiteralPath $staged -Destination $target -Force
    $deployedHash = Get-Sha256 -LiteralPath $target
    if ($deployedHash -ne $stagedHash) {
        throw 'Deployed Bridge hash does not match the compiled artifact'
    }
    Remove-Item -LiteralPath $staged -Force
} catch {
    throw "Bridge compiled successfully but the active executable could not be replaced. Pending build: $staged. Stop or restart the running Bridge, then run this script again. $($_.Exception.Message)"
}

$artifact = Get-Item -LiteralPath $target
$hash = (Get-Sha256 -LiteralPath $target).ToLowerInvariant()
Write-Host 'Task finish checks passed.' -ForegroundColor Green
Write-Host ("Bridge: {0}" -f $artifact.FullName)
Write-Host ("Built:  {0}" -f $artifact.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss'))
Write-Host ("SHA256: {0}" -f $hash)
