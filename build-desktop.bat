@echo off
echo Building maxx Wails Desktop Application...

echo Step 1: Building frontend...
cd web
call pnpm build
if errorlevel 1 (
    echo Frontend build failed!
    exit /b 1
)
cd ..

echo Step 2: Building Wails desktop app...
call wails build -platform windows/amd64
if errorlevel 1 (
    echo Wails build failed!
    exit /b 1
)

echo.
echo ========================================
echo Build completed successfully!
echo ========================================
echo.
echo Output: build\bin\maxx.exe
echo.
pause
