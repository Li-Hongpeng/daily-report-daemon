@echo off
REM daily-report-daemon Windows build script
REM Requires Go 1.19+

echo Building daily-report-daemon for Windows...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

go build -ldflags="-s -w" -o daily-report-daemon.exe ./cmd/daily-report-daemon

if %ERRORLEVEL% NEQ 0 (
    echo Build failed!
    exit /b 1
)

echo Build successful: daily-report-daemon.exe
echo.
echo Quick start:
echo   daily-report-daemon.exe init -w .
echo   daily-report-daemon.exe run -w . --dry-run
