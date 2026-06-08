# Create the release directory if it doesn't exist
New-Item -ItemType Directory -Force -Path "release" | Out-Null

Write-Host "🚀 Starting Claviger Cross-Compilation..." -ForegroundColor Cyan

# 1. LINUX UBUNTU (x86_64) - Hybrid (GUI + CLI)
Write-Host "🔨 Building Linux GUI/CLI..." -ForegroundColor Yellow
# Fyne REQUIRES CGO on Linux. You must have libgl1-mesa-dev xorg-dev installed on your build machine!
$env:CGO_ENABLED = "1" 
$env:GOOS = "linux"
$env:GOARCH = "amd64"
Remove-Item Env:\CC -ErrorAction SilentlyContinue 
go build -ldflags="-s -w" -o release/claviger-linux-amd64 .

# 2. WINDOWS (x86_64) - Hybrid
Write-Host "🔨 Building Windows GUI (with Icon)..." -ForegroundColor Yellow
$env:CGO_ENABLED = "1"
# This tells Go to use the MinGW cross-compiler you just installed via apt
$env:CC = "x86_64-w64-mingw32-gcc"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
# The .syso file you generated will automatically be picked up here!
go build -ldflags="-s -w -H=windowsgui" -o release/claviger-windows-gui.exe .

Write-Host "🔨 Building Windows CLI..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o release/claviger-windows-cli.exe .

# Cleanup
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CC -ErrorAction SilentlyContinue

Write-Host "`n✅ All builds complete! Check the 'release/' folder." -ForegroundColor Green