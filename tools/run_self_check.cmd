@echo off
setlocal
cd /d "%~dp0"
powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy RemoteSigned -File "%~dp0windows_smoke.ps1" -Portable
if errorlevel 1 (echo Self-check failed. See the details above and the report folder. & pause & exit /b 1)
echo Self-check passed. Interactive mouse operation still needs a manual check.
pause
