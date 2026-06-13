#!/bin/sh
set -e

# Stop the daemon if we are removing, purging, OR UPGRADING
if [ "$1" = "remove" ] || [ "$1" = "purge" ] || [ "$1" = "upgrade" ]; then
    echo "🛑 Stopping Claviger background daemon..."
    if [ -d /run/systemd/system ]; then
        systemctl stop claviger-client.service || true
    fi
fi