[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repo = Resolve-Path (Join-Path $PSScriptRoot '..\..')

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

$started = [Diagnostics.Stopwatch]::StartNew()
$javaHome = Resolve-JavaHome
$androidHome = Resolve-AndroidHome

$env:JAVA_HOME = $javaHome
$env:ANDROID_HOME = $androidHome
$env:ANDROID_SDK_ROOT = $androidHome
Write-BuildLog -Operation 'android.build' -Result 'started' -Context @{ javaHome = $javaHome; androidSdk = $androidHome }

& (Join-Path $PSScriptRoot 'build-go.ps1')
if ($LASTEXITCODE -ne 0) { throw "Go AAR build failed: $LASTEXITCODE" }

Push-Location $PSScriptRoot
try {
    $gradle = Join-Path $PSScriptRoot 'gradlew.bat'
    if (-not (Test-Path -LiteralPath $gradle)) { throw "Gradle Wrapper was not found: $gradle" }
    & $gradle --no-daemon assembleDebug
    if ($LASTEXITCODE -ne 0) { throw "Android APK build failed: $LASTEXITCODE" }
} finally {
    Pop-Location
}

$source = Join-Path $PSScriptRoot 'app\build\outputs\apk\debug\app-debug.apk'
$target = Join-Path $repo 'dist\downkit-android-arm64-debug.apk'
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
Copy-Item -LiteralPath $source -Destination $target -Force
$hash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
$started.Stop()
Write-BuildLog -Operation 'android.build' -Result 'succeeded' -Context @{ artifact = $target; sha256 = $hash; durationMs = $started.ElapsedMilliseconds }
