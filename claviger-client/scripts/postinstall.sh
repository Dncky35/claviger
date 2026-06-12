#!/bin/sh
set -e

echo "[+] Reloading systemd daemon..."
systemctl daemon-reload

echo "[+] Enabling and starting Claviger Gateway service..."
systemctl enable claviger-client.service
systemctl start claviger-client.service

echo "[+] Claviger installed successfully!"