@echo off
rem Double-clickable launcher (Windows): run the gdaddon binary's built-in
rem installer from this folder, then keep the console open.
cd /d "%~dp0"
gdaddon.exe install
pause
