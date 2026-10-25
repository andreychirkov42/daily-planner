@echo off
rem Двойной клик: проверки и сборка Flask.exe (см. build.ps1)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
pause
