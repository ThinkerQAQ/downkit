[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$scheme = 'HKCU:\Software\Classes\downkit'
if (Test-Path -LiteralPath $scheme) {
    Remove-Item -LiteralPath $scheme -Recurse -Force
}
foreach ($browser in @('Google\Chrome', 'Microsoft\Edge')) {
    $nativeKey = "HKCU:\Software\$browser\NativeMessagingHosts\com.downkit.bridge"
    if (Test-Path -LiteralPath $nativeKey) {
        Remove-Item -LiteralPath $nativeKey -Recurse -Force
    }
}
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
$nativeManifest = Join-Path $env:LOCALAPPDATA 'DownKit\com.downkit.bridge.json'
if (Test-Path -LiteralPath $nativeManifest) {
    Remove-Item -LiteralPath $nativeManifest -Force
}
Write-Host 'downkit: 协议和 Native Messaging Host 已移除。' -ForegroundColor Green
