@echo off
setlocal enabledelayedexpansion

REM Get the directory where the batch file is located
set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

echo Working directory: %SCRIPT_DIR%

REM Check if Go is installed
go version >nul 2>&1
if !errorlevel! neq 0 (
    echo Error: Go is not installed or not in the PATH
    echo Please install Go from https://golang.org/doc/install
    echo.
    echo After installation, make sure to:
    echo 1. Restart your command prompt
    echo 2. Verify Go is in your PATH by running: go version
    pause
    exit /b 1
)

echo Go is installed:
go version
echo Creating Go module for Netdata MCP bridge...

REM Initialize Go module if it doesn't exist
if not exist "go.mod" (
    go mod init netdata/nd-mcp-bridge
    echo Initialized new Go module
) else (
    echo Go module already exists
)

REM Add required dependencies
echo Adding dependencies...
go get github.com/coder/websocket
go mod tidy

REM Build the binary
echo Building nd-mcp.exe binary...
go build -o nd-mcp.exe nd-mcp.go

if !errorlevel! neq 0 (
    echo Build failed!
    pause
    exit /b 1
)

REM Get the full path to the executable
set "EXECUTABLE_PATH=%SCRIPT_DIR%nd-mcp.exe"

echo.
echo Build complete!
echo.
echo You can now run the bridge using the full path:
echo "%EXECUTABLE_PATH%" ws://ip:19999/mcp
echo.
echo Or from this directory:
echo nd-mcp.exe ws://ip:19999/mcp
echo.
echo Example usage:
echo nd-mcp.exe ws://localhost:19999/mcp
echo nd-mcp.exe ws://192.168.1.100:19999/mcp
echo.
pause