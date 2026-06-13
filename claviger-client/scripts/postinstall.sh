#!/bin/sh
set -e

if [ "$1" = "configure" ]; then
    echo "✅ Configuring Claviger..."
    if [ -d /run/systemd/system ]; then
        systemctl daemon-reload || true
        systemctl enable claviger-client.service || true
        systemctl restart claviger-client.service || true
    fi
fi