#!/bin/sh
set -e

# 1. Always reload systemd so it forgets the deleted .service file
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi

# 2. Check what kind of uninstallation the user requested
if [ "$1" = "purge" ]; then
    # Scorched Earth: They ran 'apt-get purge' or selected "Completely Remove"
    echo "🔥 Purging Claviger: Wiping Vault and configuration files..."
    rm -rf /etc/claviger
    
elif [ "$1" = "remove" ]; then
    # Standard Remove: Keep the Vault safe just in case they reinstall
    echo "⚠️  Claviger application was removed, but your profiles in /etc/claviger were kept safe."
    echo "To completely erase all data, run: sudo apt-get purge claviger-client"
fi