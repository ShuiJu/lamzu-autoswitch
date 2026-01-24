@echo off
:: Delete old exe
if exist "lamzu-autoswitch.exe" (
    echo Deleting older version...
    del /f "lamzu-autoswitch.exe"
)

:: compile
echo Compiling...
go build -trimpath -ldflags "-s -w" -o lamzu-autoswitch.exe

:: Check if success
if errorlevel 1 (
    echo success
    pause
    exit /b 1
)

echo Finished
if exist "lamzu-autoswitch.exe" pause
