@echo off
setlocal
cd /d "%~dp0"
powershell.exe -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0build.ps1"
if errorlevel 1 (echo Build failed. See the message above. & pause & exit /b 1)
echo Built dist\ApplyKit.exe
pause
