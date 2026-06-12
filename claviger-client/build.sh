#!/bin/bash

# Create the release directory if it doesn't exist
mkdir -p release

echo "🚀 Starting Claviger Cross-Compilation..."

# 1. LINUX UBUNTU (x86_64) - Hybrid (GUI + CLI)
echo "🔨 Building Linux GUI/CLI (x86_64)..."
# Fyne REQUIRES CGO on Linux. Ensure libgl1-mesa-dev and xorg-dev are installed!
env -u CC CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o release/claviger-linux-amd64 .

# 2. WINDOWS (x86_64) - Hybrid
echo "🔨 Building Windows GUI (with Icon)..."
# This uses the MinGW cross-compiler for Windows C-bindings
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o release/claviger-windows-gui.exe .

echo "🔨 Building Windows CLI..."
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o release/claviger-windows-cli.exe .

# 3. ARM SYSTEMS (Linux ARM64) - PURE HEADLESS
echo "🔨 Building Linux ARM64 (Headless Only)..."
# Zero C-Dependencies needed for pure CLI! Compiles purely in Go!
env -u CC CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags=headless -ldflags="-s -w" -o release/claviger-linux-arm64-cli .

echo "📦 Packaging Linux Distributions with nFPM..."

# Ensure nfpm is installed on your build machine:
# go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

cd packaging

# Build Debian (.deb) packages
nfpm pkg --config nfpm-amd64.yaml --target ../release/claviger_1.0.0_amd64.deb
nfpm pkg --config nfpm-arm64.yaml --target ../release/claviger_1.0.0_arm64.deb

# Build RHEL/CentOS (.rpm) packages
nfpm pkg --config nfpm-amd64.yaml --target ../release/claviger_1.0.0_amd64.rpm
nfpm pkg --config nfpm-arm64.yaml --target ../release/claviger_1.0.0_arm64.rpm

cd ..
echo "✅ Packaging complete! .deb and .rpm files are in the release/ folder."