[CmdletBinding()]
param(
    [string]$Executable = '',
    [string[]]$ExtensionId = @('hfjpenjlamneepmigemmmiibebpokmik')
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$releaseExecutable = Join-Path $root 'downkit-windows-amd64.exe'
$sourceExecutable = Join-Path $root 'dist\downkit-windows-amd64.exe'
$exe = if ($Executable) {
    $Executable
} elseif (Test-Path -LiteralPath $releaseExecutable) {
    $releaseExecutable
} else {
    $sourceExecutable
}
$exe = [System.IO.Path]::GetFullPath($exe)
if (-not (Test-Path -LiteralPath $exe)) {
    throw "找不到程序：$exe"
}

$extensionIds = @(
    $ExtensionId |
        ForEach-Object { $_.Trim().ToLowerInvariant() } |
        Where-Object { $_ } |
        Select-Object -Unique
)
if (-not $extensionIds) {
    throw 'At least one extension ID is required. Usage: .\Install-Windows.ps1 -ExtensionId <extension_id>'
}
foreach ($id in $extensionIds) {
    if ($id -notmatch '^[a-p]{32}$') {
        throw "Invalid Chromium extension ID: $id"
    }
}

$scheme = 'HKCU:\Software\Classes\downkit'
$commandKey = Join-Path $scheme 'shell\open\command'
$command = '"{0}" "%1"' -f $exe

New-Item -Path $scheme -Force | Out-Null
Set-Item -Path $scheme -Value 'URL:DownKit Protocol'
New-ItemProperty -Path $scheme -Name 'URL Protocol' -Value '' -PropertyType String -Force | Out-Null
New-Item -Path $commandKey -Force | Out-Null
Set-Item -Path $commandKey -Value $command

$nativeDir = Join-Path $env:LOCALAPPDATA 'DownKit'
$nativeManifest = Join-Path $nativeDir 'com.downkit.bridge.json'
New-Item -ItemType Directory -Path $nativeDir -Force | Out-Null
$allowedOrigins = [System.Collections.Generic.SortedSet[string]]::new([System.StringComparer]::Ordinal)
foreach ($id in $extensionIds) {
    [void]$allowedOrigins.Add("chrome-extension://$id/")
}
# Like pflow, keep extension IDs registered by another Chromium browser/build.
if (Test-Path -LiteralPath $nativeManifest) {
    try {
        $existing = Get-Content -LiteralPath $nativeManifest -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($existing.name -eq 'com.downkit.bridge') {
            foreach ($origin in @($existing.allowed_origins)) {
                if ($origin -is [string] -and $origin -match '^chrome-extension://[a-p]{32}/$') {
                    [void]$allowedOrigins.Add($origin)
                }
            }
        }
    } catch {
        Write-Warning "Ignoring an unreadable Native Host manifest: $nativeManifest"
    }
}
$nativeHostConfig = [ordered]@{
    name = 'com.downkit.bridge'
    description = 'DownKit local bridge'
    path = $exe
    type = 'stdio'
    allowed_origins = @($allowedOrigins)
}
$json = $nativeHostConfig | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText($nativeManifest, $json, [System.Text.UTF8Encoding]::new($false))

$registeredBrowsers = @()
foreach ($browser in @('Google\Chrome', 'Microsoft\Edge')) {
    $nativeKey = "HKCU:\Software\$browser\NativeMessagingHosts\com.downkit.bridge"
    New-Item -Path $nativeKey -Force | Out-Null
    Set-Item -Path $nativeKey -Value $nativeManifest
    $registeredPath = (Get-Item -Path $nativeKey).GetValue('')
    if ($registeredPath -ne $nativeManifest) {
        throw "Native Host registry verification failed: $nativeKey"
    }
    $registeredBrowsers += $browser
}

# Remove registrations from the pre-DownKit name. Files are intentionally kept
# so an old build can still be restored manually if needed.
$legacyScheme = 'HKCU:\Software\Classes\catchhlsgo'
if (Test-Path -LiteralPath $legacyScheme) {
    Remove-Item -LiteralPath $legacyScheme -Recurse -Force
}
foreach ($browser in @('Google\Chrome', 'Microsoft\Edge')) {
    $legacyNativeKey = "HKCU:\Software\$browser\NativeMessagingHosts\com.catchhls.go"
    if (Test-Path -LiteralPath $legacyNativeKey) {
        Remove-Item -LiteralPath $legacyNativeKey -Recurse -Force
    }
}

Write-Host 'downkit: 协议和 Native Messaging Host 已注册。' -ForegroundColor Green
Write-Host $command
Write-Host $nativeManifest
Write-Host ('Allowed extension IDs: ' + ($extensionIds -join ', '))
Write-Host ('Registered browsers: ' + ($registeredBrowsers -join ', '))
