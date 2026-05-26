#!/bin/bash

mkdir -p release
echo "🚀 Starting Claviger Cross-Compilation..."

# 1. LINUX MINT (x86_64) - Hybrid (GUI + CLI)
echo "🔨 Building Linux (x86_64)..."
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o release/claviger-linux-amd64 .

# 2. WINDOWS (x86_64) - Hybrid
echo "🔨 Building Windows GUI..."
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o release/claviger-windows-gui.exe .

echo "🔨 Building Windows CLI..."
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o release/claviger-windows-cli.exe .

# 3. ARM SYSTEMS (Linux ARM64) - PURE HEADLESS
echo "🔨 Building Linux ARM64 (Headless Only)..."
# 🎯 Zero C-Dependencies needed! Compiles purely in Go!
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags=headless -ldflags="-s -w" -o release/claviger-linux-arm64-cli .

echo ""
echo "✅ All builds complete! Look inside the 'release/' folder."