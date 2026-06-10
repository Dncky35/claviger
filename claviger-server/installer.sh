#!/bin/bash
set -e # Exit immediately if any command fails

# Replace this with your actual GitHub repository URL
DEST_PATH="/usr/local/bin/claviger-server"

echo "🚀 Starting Claviger Server Installation..."

# 1. Require Root (sudo) Privileges
if [ "$EUID" -ne 0 ]; then
  echo "❌ Error: Please run this script with root privileges."
  echo "Try: curl -sSL https://your-install-url.com | sudo bash"
  exit 1
fi

# 2. Detect System Architecture
ARCH=$(uname -m)
echo "🔍 Detected Architecture: $ARCH"

if [ "$ARCH" = "x86_64" ]; then
    FILE_NAME="claviger-server-amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    FILE_NAME="claviger-server-arm64"
else
    echo "❌ Error: Unsupported architecture: $ARCH"
    echo "Claviger currently supports x86_64 and arm64."
    exit 1
fi

DOWNLOAD_URL="https://github.com/Dncky35/claviger/releases/download/Claviger/$FILE_NAME"

# 3. Download the Binary
echo "⬇️ Fetching the latest Claviger release..."
echo "🔗 Downloading from: $DOWNLOAD_URL"

# We use curl with -f (fail silently on HTTP errors) so it doesn't write a 404 page to the binary file
if ! curl -sSLf -o "$DEST_PATH" "$DOWNLOAD_URL"; then
    echo "❌ Error: Failed to download the binary. Please check the GitHub repository and release names."
    exit 1
fi

# 4. Make it Executable
echo "🔑 Applying executable permissions..."
chmod +x "$DEST_PATH"

# 5. Hand over control to the Go Application!
echo "⚙️ Initializing Claviger Systemd Integration..."
# This triggers your Go code (InstallSystemdService)
claviger-server setup

# echo ""
# echo "======================================================="
# echo "✅ SUCCESS: Claviger Edge VPN Daemon is now installed!"
# echo "======================================================="
# echo "You can manage the service anytime using:"
# echo "  sudo systemctl status claviger"
# echo "  sudo systemctl restart claviger"
# echo ""
# echo "🌐 Next Step: Open your browser to complete setup at:"
# echo "   http://<YOUR_SERVER_IP>:18080"
# echo "======================================================="