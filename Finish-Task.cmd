@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Finish-Task.ps1" %*
exit /b %ERRORLEVEL%
