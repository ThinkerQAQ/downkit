[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$bridge = Join-Path $root 'bridge'
$output = Join-Path $PSScriptRoot 'app\libs\downkit.aar'
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $output) | Out-Null

function Write-BuildLog {
    param(
        [Parameter(Mandatory = $true)][string]$Operation,
        [Parameter(Mandatory = $true)][string]$Result,
        [hashtable]$Context = @{}
    )
    $record = [ordered]@{
        timestamp = [DateTimeOffset]::UtcNow.ToString('o')
        severity = 'INFO'
        operation = $Operation
        result = $Result
    }
    foreach ($key in $Context.Keys) { $record[$key] = $Context[$key] }
    Write-Host ($record | ConvertTo-Json -Compress)
}

function Resolve-JavaHome {
    if ($env:JAVA_HOME -and (Test-Path -LiteralPath (Join-Path $env:JAVA_HOME 'bin\javac.exe'))) {
        return $env:JAVA_HOME
    }
    $javac = Get-Command javac.exe -ErrorAction SilentlyContinue
    if ($javac) { return Split-Path -Parent (Split-Path -Parent $javac.Source) }
    throw 'JDK 17 or newer was not found. Set JAVA_HOME or add javac to PATH.'
}

function Resolve-AndroidHome {
    foreach ($candidate in @($env:ANDROID_SDK_ROOT, $env:ANDROID_HOME, (Join-Path $env:LOCALAPPDATA 'Android\Sdk'))) {
        if ($candidate -and (Test-Path -LiteralPath $candidate)) { return $candidate }
    }
    throw 'Android SDK was not found. Set ANDROID_SDK_ROOT or ANDROID_HOME.'
}

function Resolve-AndroidNdkHome([string]$AndroidHome) {
    if ($env:ANDROID_NDK_HOME -and (Test-Path -LiteralPath (Join-Path $env:ANDROID_NDK_HOME 'source.properties'))) {
        return $env:ANDROID_NDK_HOME
    }
    $ndkRoot = Join-Path $AndroidHome 'ndk'
    $candidate = Get-ChildItem -LiteralPath $ndkRoot -Directory -ErrorAction SilentlyContinue |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'source.properties') } |
        Sort-Object Name -Descending |
        Select-Object -First 1
    if ($candidate) { return $candidate.FullName }
    throw 'Android NDK was not found. Set ANDROID_NDK_HOME or install an NDK in the Android SDK.'
}

$started = [Diagnostics.Stopwatch]::StartNew()

$goPath = (& go env GOPATH).Trim()
$gomobile = Join-Path $goPath 'bin\gomobile.exe'
$gobind = Join-Path $goPath 'bin\gobind.exe'
if (-not (Test-Path -LiteralPath $gomobile)) {
    throw "gomobile not found: $gomobile"
}
if (-not (Test-Path -LiteralPath $gobind)) {
    throw "gobind not found: $gobind"
}
$goMod = Get-Content -LiteralPath (Join-Path $bridge 'go.mod') -Raw
$mobileVersionMatch = [regex]::Match($goMod, 'golang\.org/x/mobile\s+(v\S+)')
if (-not $mobileVersionMatch.Success) { throw 'golang.org/x/mobile version was not found in bridge/go.mod' }
$requiredMobileVersion = $mobileVersionMatch.Groups[1].Value
foreach ($tool in @($gomobile, $gobind)) {
    $metadata = (& go version -m $tool) -join "`n"
    $toolVersionMatch = [regex]::Match($metadata, '(?m)^\s*mod\s+golang\.org/x/mobile\s+(v\S+)')
    if (-not $toolVersionMatch.Success -or $toolVersionMatch.Groups[1].Value -ne $requiredMobileVersion) {
        throw "$(Split-Path -Leaf $tool) does not match bridge/go.mod. Install version $requiredMobileVersion as documented in mobile/android/README.md."
    }
}

$javaHome = Resolve-JavaHome
$androidHome = Resolve-AndroidHome
$androidNdkHome = Resolve-AndroidNdkHome $androidHome

$env:JAVA_HOME = $javaHome
$env:ANDROID_HOME = $androidHome
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:ANDROID_NDK_HOME = $androidNdkHome
$env:PATH = "$(Join-Path $goPath 'bin');$(Join-Path $env:JAVA_HOME 'bin');$env:PATH"
Write-BuildLog -Operation 'android.go-bind' -Result 'started' -Context @{ javaHome = $javaHome; androidSdk = $androidHome; androidNdk = $androidNdkHome; gomobile = $gomobile }

foreach ($required in @(
    (Join-Path $env:JAVA_HOME 'bin\javac.exe'),
    (Join-Path $env:ANDROID_HOME 'platforms\android-35\android.jar'),
    (Join-Path $env:ANDROID_NDK_HOME 'source.properties')
)) {
    if (-not (Test-Path -LiteralPath $required)) { throw "Missing Android build dependency: $required" }
}

Push-Location $bridge
try {
    & $gomobile bind -target android/arm64 -androidapi 26 -o $output .
    if ($LASTEXITCODE -ne 0) { throw "gomobile bind failed: $LASTEXITCODE" }
} catch {
    $started.Stop()
    Write-BuildLog -Operation 'android.go-bind' -Result 'failed' -Context @{ durationMs = $started.ElapsedMilliseconds; errorType = $_.Exception.GetType().FullName; message = $_.Exception.Message }
    throw
} finally {
    Pop-Location
}

$hash = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant()
$started.Stop()
Write-BuildLog -Operation 'android.go-bind' -Result 'succeeded' -Context @{ artifact = $output; sha256 = $hash; durationMs = $started.ElapsedMilliseconds }
