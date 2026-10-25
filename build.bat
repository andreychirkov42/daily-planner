@echo off
rem Двойной клик: проверки и сборка Clepsy.exe (см. build.ps1)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
pause
