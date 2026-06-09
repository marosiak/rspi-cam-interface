#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="timelapse.service"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (e.g., sudo ./logs.sh)"
    exit 1
fi

echo "=== Status: ${SERVICE_NAME} ==="
systemctl status "${SERVICE_NAME}" --no-pager

echo ""
echo "=== Logs: ${SERVICE_NAME} ==="
journalctl -u "${SERVICE_NAME}" --no-pager -n 50
