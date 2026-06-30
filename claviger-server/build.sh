#!/bin/bash
set -e # Exit immediately if any command fails

# Define the output directory
OUT_DIR="release"

# Create the release folder and clean it if it already exists
mkdir -p $OUT_DIR
rm -f $OUT_DIR/*

echo "🚀 Starting Claviger Server Compilation..."

# ---------------------------------------------------------
# 1. LINUX AMD64 (Standard Intel/AMD Servers like DigitalOcean/AWS)
# ---------------------------------------------------------
echo "🔨 Building Linux AMD64 (x86_64)..."
# CGO_ENABLED=0 creates a 100% statically linked binary. 
# It will run on ANY Linux distro without needing dependencies!
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $OUT_DIR/claviger-server-amd64 .

# ---------------------------------------------------------
# 2. LINUX ARM64 (AWS Graviton, Oracle ARM, Raspberry Pi 4/5)
# ---------------------------------------------------------
echo "🔨 Building Linux ARM64 (aarch64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $OUT_DIR/claviger-server-arm64 .

# ---------------------------------------------------------
# Generate Checksums (Unified for the Auto-Updater)
# ---------------------------------------------------------
echo "🔏 Generating SHA256 Checksums..."
cd $OUT_DIR
# Grab hashes of all built binaries and put them into a single file
sha256sum * > checksums-server.txt
cd ..

echo ""
echo "✅ Server builds complete! Binaries and checksums.txt are in the '$OUT_DIR/' folder."