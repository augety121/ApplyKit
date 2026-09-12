@echo off
setlocal
cd /d "%~dp0"
echo ApplyKit 2.1.0 Windows self-check
powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy RemoteSigned -File "%~dp0windows_smoke.ps1" -Portable
if errorlevel 1 (
  echo.
  echo Self-check FAILED. Please keep the generated acceptance folder for diagnosis.
  pause
  exit /b 1
)
echo.
echo Self-check PASSED. An acceptance folder was created on your Desktop or Temp folder.
pause
