@echo off
setlocal

set "OUTPUT_EXE=lamzu-autoswitch-V0.4.1.exe"
set "RSRC_EXE=%USERPROFILE%\go\bin\rsrc.exe"
set "SYSO_FILE=rsrc_windows_amd64.syso"

if exist "%OUTPUT_EXE%" (
    echo Deleting older version...
    del /f "%OUTPUT_EXE%"
)

if exist "rsrc.syso" del /f "rsrc.syso"
if exist "%SYSO_FILE%" del /f "%SYSO_FILE%"

if exist "%RSRC_EXE%" goto have_rsrc
echo rsrc.exe not found: %RSRC_EXE%
exit /b 1

:have_rsrc
echo Building Windows icon resources...
"%RSRC_EXE%" -arch amd64 -ico "lamzu-icon.ico" -o "%SYSO_FILE%"
if errorlevel 1 (
    echo Failed to build icon resource.
    exit /b 1
)

echo Compiling...
go build -buildvcs=false -trimpath -ldflags "-s -w -H=windowsgui" -o "%OUTPUT_EXE%"

if errorlevel 1 (
    echo Build failed.
    pause
    exit /b 1
)

echo Finished
