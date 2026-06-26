#!/bin/bash
set -e # Exit immediately if a command fails

# Create the directories if they don't exist
mkdir -p build
mkdir -p release

echo "🚀 Starting Claviger Cross-Compilation..."

# 1. LINUX UBUNTU (x86_64) - Hybrid (GUI + CLI)
echo "🔨 Building Linux GUI/CLI (x86_64)..."
# Fyne REQUIRES CGO on Linux. Ensure libgl1-mesa-dev and xorg-dev are installed!
env -u CC CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=headless -ldflags="-s -w" -o build/claviger-client-amd64 .

# 2. WINDOWS (x86_64) - Hybrid
echo "🔨 Building Windows GUI (with Icon)..."
# This uses the MinGW cross-compiler for Windows C-bindings
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o build/claviger-windows-gui.exe .

echo "🔨 Building Windows CLI..."
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build/claviger-windows-cli.exe .

# 3. ARM SYSTEMS (Linux ARM64) - PURE HEADLESS
echo "🔨 Building Linux ARM64 (Headless Only)..."
# Zero C-Dependencies needed for pure CLI! Compiles purely in Go!
env -u CC CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags=headless -ldflags="-s -w" -o build/claviger-client-arm64 .

echo "📦 Packaging Linux Distributions with nFPM..."

# Ensure nfpm is installed on your build machine:
# go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

cd packaging

# Build Debian (.deb) packages
nfpm pkg --config nfpm-amd64.yaml --target ../release/claviger_amd64.deb
nfpm pkg --config nfpm-arm64.yaml --target ../release/claviger_arm64.deb

# Build RHEL/CentOS (.rpm) packages
nfpm pkg --config nfpm-amd64.yaml --target ../release/claviger_amd64.rpm
nfpm pkg --config nfpm-arm64.yaml --target ../release/claviger_arm64.rpm

cd ..
echo "✅ Packaging complete!"

# --- NEW: CHECKSUM GENERATION ---
echo "🔐 Generating SHA256 Checksums for Release..."

cd release
# Generate hashes for all files in the release directory and save to checksums.txt
sha256sum * > checksums.txt
cd ..

echo "✅ Build, Packaging, and Security Checksums are 100% complete!"
echo "📂 Your deployment files are ready in the 'release/' folder."